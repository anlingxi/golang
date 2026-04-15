# RAG 亮点一：文件上传与异步文档处理 — 面试深挖整理

> 亮点原文：设计上传-处理分离架构，前端分片直传 MinIO（支持秒传与断点续传），上传完成立即返回；合并后 Kafka 异步驱动文档处理 Pipeline（解析 → 切分 → 向量化 → 入库），避免耗时操作阻塞用户请求。

---

## 一、分片上传

### Q：分片大小怎么定的？

**A：** 固定 5MB。参考 MinIO 推荐范围（5MB–5GB/part），同时知识库文档普遍在几十 MB 以内，5MB 分片数少、开销小。

**延伸追问：小文件是否也分片？**
- 当前是固定大小，小文件也走同样流程，未做动态分片阈值。生产上可以补一个阈值判断：小于 5MB 直传，不走分片逻辑。

---

### Q：秒传原理？

**A：** 上传前先调 `checkfile` 接口，服务端检查 `user_id + file_md5` 在数据库是否已存在且 `status = 完成`。若命中，返回"已完成"信号，前端直接调 `fastupload` 接口（二次确认 MySQL）。

**追问：是全局去重还是按用户隔离？**
- **按用户隔离**，不做全局秒传。原因是多租户场景下，如果全局去重，用户 A 删除文件会导致全局对象被清理，用户 B 再也找不到他上传的记录，产生越权隐患。
- 代价：同一文件跨用户无法复用，严格意义上不叫"秒传"，更准确的描述是"同用户重复上传免重传"。

**追问：`checkfile → fastupload` 两步有竞态窗口，如何幂等？**
- 利用数据库 `UNIQUE(user_id, file_md5)` 唯一索引。两个并发请求同时 check 都未命中，谁先写入谁成功，另一个报键冲突并被挡住。这是数据库层面的最终幂等保障，不依赖应用层锁。

---

### Q：断点续传怎么实现的？"断点"状态存在哪里？

**A：**
- Redis 是**加速层**，记录分片上传状态（快速读写）。
- MySQL 是**真相层**，持久化每个分片的元数据（chunk_index、存储路径、上传状态）。
- 服务重启或 Redis Key 过期（`redis.nil`）时，降级读 MySQL，重建分片进度。
- 中断恢复流程：客户端先向服务端查询已完成分片列表 → 服务端从 Redis 或 MySQL 返回 → 客户端**只补传缺失分片**，不重传整文件。

**追问：这叫"断点续传"吗，不就是客户端重试？**
- 断点续传的本质不是"服务器把分片重新发给客户端"，而是"服务器记住进度，客户端只补传缺失片"。所以仍属于断点续传，更准确称呼是**应用层分片续传**，区别于 MinIO 原生 Multipart Upload（存储层协议）。

**追问：为什么不用 MinIO 原生 Multipart Upload？**
- 我们的方案：每个分片作为独立对象上传，服务端合并。好处是状态管理完全在应用层，不受 UploadID 生命周期约束。
- 如果使用原生 Multipart：UploadID **必须持久化到数据库**。服务重启后要靠 UploadID 去 `list parts`，继续 `complete multipart upload`。若 UploadID 丢失，已上传 parts 变成悬挂分片，需要 `abort multipart upload` 或生命周期策略清理。

---

## 二、分片合并

### Q：合并是下载到服务器拼接，还是 MinIO 服务端合并？

**A：** 使用 MinIO 的 `ComposeObject` / `CopyObject` 做**服务端合并**。应用服务器只发一条"合并这些对象"的指令，数据拼接在 MinIO 侧完成，100MB 文件不会再流经业务机。

**代码位置：**
- 单分片：`upload_service.go:241` 走 `CopyObject`
- 多分片：`upload_service.go:258` 走 `ComposeObject`

这保证了"上传-处理分离"在合并这步也成立——数据面不经过应用服务器。

---

### Q：合并后分片对象如何清理？如果清理失败怎么办？

**当前实现（基础版）：** 合并完成后异步 `best-effort` 删除分片对象，失败只打日志，无重试、无补偿。

**生产级做法：**
1. 保留"合并后立即删除"作为最快路径。
2. 补 **MinIO Bucket Lifecycle / ILM** 作为兜底：

```bash
mc ilm rule add --prefix "chunks/" --expire-days 7 myminio/mybucket
```

按 `chunks/` 前缀设过期规则，即使应用删除失败，生命周期扫描最终也会收走孤儿分片，不会误删最终文件（`merged/` 前缀不受影响）。

> **注意：** MinIO ILM 是由后台 scanner 异步执行，不保证"立刻删除"，是最终一致清理。

---

### Q：MinIO 对象 key 有没有做用户隔离？

**当前代码存在的问题：**
- 分片 key：`chunks/{fileMD5}/{chunkIndex}` — 没带 `userID`
- 合并后 key：`merged/{fileName}` — 没带 `userID`

**风险：** 不同用户上传同名文件，`merged/{fileName}` 会发生**覆盖**。

**正确设计：**
```
merged/{userID}/{fileMD5}/{fileName}
chunks/{userID}/{fileMD5}/{chunkIndex}
```

---

## 三、Kafka 触发与可靠性

### Q：Kafka 发送是同步还是异步的？

**A：** 同步发送（`producer.WriteMessages(...)` 等待 broker ack 后返回）。

**代码位置：**
- `upload_service.go:287` 调用 `kafka.ProduceFileTask(task)`
- `pkg/kafka/client.go:36` 内部同步执行

---

### Q：合并成功、Kafka 失败，只打日志——后果是什么？

**当前状态（裂脑问题）：**
- `upload_service.go:280`：合并成功后先把数据库状态改成 `status=1`（上传完成）
- `upload_service.go:298`：Kafka 失败只记日志，不回滚，接口仍返回成功

**结果：** 文件在 MinIO 存着，数据库显示"上传完成"，Pipeline 永远不会触发，知识库无内容，无自动恢复。

---

### Q：如何保证"合并成功则 Pipeline 一定被触发"？

**方案一：Outbox Pattern（标准方案）**

```
合并 MinIO 对象成功
    ↓
开启 DB 事务
  ├── 更新文件状态 process_status = pending
  └── 插入 outbox 事件（payload = Kafka 消息内容）
提交事务
    ↓
独立 relay/dispatcher 进程轮询 outbox 表
  ├── 投递 Kafka 成功 → 标记 outbox.status = sent
  └── 失败 → 退避重试 → 超次进死信队列
```

**核心保障：** 事务原子性保证"DB 更新 + 事件入库"同时成功或失败，Kafka 投递失败不影响事件持久化，relay 负责最终投递。

**Outbox 的隐藏问题 — 幂等性：**
Relay 投递成功但在标记 `sent` 前崩溃 → 下次轮询重投 → Kafka 里有两条相同消息。因此 **Pipeline 消费者必须幂等**（见第四节）。

**方案二：轻量补偿（次优但务实）**
- 合并成功后先把文件标记为 `pending`，Kafka 成功后再推进状态
- Kafka 失败时标记为 `dispatch_failed`，不返回"完全成功"
- 增加**定时补偿任务**，扫描 `process_status IN (pending, dispatch_failed)` 的记录，重新发送 Kafka 消息

**轻量方案的漏洞：** 如果没有定时扫描任务，`pending` 只是"状态更好看"的静默失败，文件依然卡死。

---

## 四、Pipeline 消费端可靠性

### Q：Kafka 消息手动提交，重试三次失败后直接提交——后果是什么？

**A：**
- 超过重试次数后，文件状态更新为 `failed`，Kafka offset 提交，消息丢弃。
- 用户侧：文件显示"处理失败"，但没有通知推送。
- **当前缺口：** 没有死信队列（DLQ），无法事后排查哪些文件失败、原因是什么。

**生产级补充：** 超次后把消息投入 DLQ（独立 Kafka topic），运维可以查看并手动重发，同时触发告警通知。

---

### Q：Pipeline 消费者幂等吗？同一文件重复消费会怎样？

**当前代码问题（基于 `pipeline/service.go`）：**
- `Process()` 每次都重新读 MinIO、重新解析、重新切分
- `persistChunks()` 直接 `docVectorRepo.Create(record)`，无"已存在则跳过/覆盖"逻辑
- `document_vectors` 表只有自增主键，无 `(user_id, file_md5, chunk_id)` 唯一约束
- 仓库层 `Create()` 是直接 INSERT，不是 upsert

**结果：** 同一文件被消费两次 → `document_vectors` 重复插入 → ES 也可能重复写入。

---

### Q：如何三层保证 Pipeline 幂等？

**第一层：消费前防重（CAS 状态流转）**

```sql
UPDATE file_upload
SET process_status = 1  -- processing
WHERE file_md5 = ? AND user_id = ? AND process_status IN (0, 3)
-- 0=pending, 3=failed
```

检查 affected rows：= 0 说明已被抢占或已完成，直接 ack 跳过。这是**原子操作**，不存在 SELECT + UPDATE 的竞态。

**代码实现（已落地）：** `upload_repository.go` 的 `TryMarkFileProcessing`，在 `pipeline/service.go:Process()` 开头调用。

```go
func (s *Service) Process(ctx context.Context, req ProcessRequest) (result *ProcessResult, err error) {
    acquired, err := s.uploadRepo.TryMarkFileProcessing(req.FileMD5, req.UserID)
    if err != nil { return nil, err }
    if !acquired {
        log.Infof("[DocumentPipeline] skip duplicated task file_md5=%s", req.FileMD5)
        return &ProcessResult{...}, nil
    }
    // defer 回写 completed / failed
    ...
}
```

**第二层：数据库落库幂等写**

给 `document_vectors` 增加唯一键：

```sql
UNIQUE(user_id, file_md5, chunk_id)
-- 如果考虑模型升级：
UNIQUE(user_id, file_md5, chunk_id, model_version)
```

`persistChunks()` 改为 upsert（已存在则 UPDATE，不存在才 INSERT）。

**第三层：ES 使用稳定文档 ID**

ES 幂等的关键是"重复发也覆盖同一条文档"。文档 ID 固定为：

```
hash(userID:fileMD5:chunkID:modelVersion)
```

同一 chunk 重复写入 → 覆盖，不新增。需改造 eino indexer 封装，显式传入文档 ID。

---

## 五、Kafka 分步状态与重试

### Q：Pipeline 四步中某步失败，消息状态是什么？如何重试？

**A（当前实现）：**
- 手动提交 offset，失败时在 offset 提交之前进行重试
- 重试三次失败 → 提交 offset（消息不再重投）→ 状态标记 `failed`
- 解析和切分每次重试都重复执行，**没有分步状态记录**（即无法从失败步骤续跑）

**当前 stage 追踪（已落地）：**
```go
stage := "fetching"
defer func() {
    if err != nil {
        s.uploadRepo.MarkFileProcessingFailed(req.FileMD5, req.UserID, stage, err.Error())
    }
}()
stage = "parsing"   // 进入解析
stage = "transforming" // 进入切分
stage = "persisting"   // 进入落库
stage = "indexing"     // 进入 ES
```

这样失败时至少知道是哪个阶段出的问题，但重试仍从头开始。

---

## 六、切分策略

### Q：用什么切分方式？有没有 overlap？

**A：**
- **递归文本切分**（Recursive Text Splitter）：优先按段落 `\n\n`、换行 `\n`、句号、空格等自然边界切分，边界不够才退化到字符级。
- **Overlap：chunk 大小的 10%**，目的是减少边界处信息截断（语义连续性）。
- 对 PDF 中的**表格和图片**：当前只提取文本，表格结构和图片内容不做单独建模，是已知局限。

---

## 七、向量化

### Q：用哪个 embedding 模型？批量还是逐条？

**A：**
- 模型：阿里 `text-embedding-v4`，向量维度 **2048**
- 批量处理，`batch_size = 10`（保守默认值，应基于 API 压测调整）
- 外部 API 限流/超时处理当前不完善，生产上需补**指数退避 + 熔断**

---

## 八、存储与检索

### Q：向量存在哪里？ES 用什么字段类型？

**A：** 所有数据存 Elasticsearch：
- 向量：`dense_vector` 字段，开启 `index: true`，走 **kNN API（HNSW 近似检索）**，非全量扫描
- 文本：`text` 字段，配置 **IK 分词器**

**IK 分词模式选择：**
- **索引时**：`ik_max_word`（细粒度，切更多词，提升召回）
- **查询时**：`ik_smart`（粗粒度，保留自然词组，避免语义拆碎）

如果两端都用细粒度，"知识库"会被拆成"知识"+"库"两个 token，查询"知识库管理"会失配。如果都用粗粒度，索引里词条太少，召回率下降。

---

### Q：混合检索怎么做的？RRF 是什么？为什么选 RRF？

**A：** 两路检索：
1. **kNN（dense_vector）**：语义相似度
2. **BM25（text + IK）**：关键词匹配

**RRF（Reciprocal Rank Fusion）融合公式：**

$$\text{score}(d) = \sum_{r \in R} \frac{1}{k + \text{rank}_r(d)}$$

- 对每一路检索结果，按排名取倒数（加平滑参数 k，通常 k=60）
- 对每个文档在各路中的 RRF 分数求和

**为什么选 RRF 而不是加权求和：**
- 加权求和需要针对不同数据集调参（语义权重多少、BM25 权重多少），不够通用
- RRF 无需调参，结果稳定，是"不会出错的基准选择"

---

### Q：HNSW 参数有没有调过？

**A：** 当前使用 ES 默认值，未针对业务数据调优。

**关键参数：**
- `m`：每个节点的连接数，越大精度越高，内存越大
- `ef_construction`：建索引时的 beam width，越大索引质量越高，构建越慢
- `num_candidates`：查询时的候选池大小，建议设为目标 k 的 10 倍

**`num_candidates` 的 trade-off：**
- 过大 → 每次查询扫描节点多，延迟升高
- 过小 → 候选池不足，真正相似的文档未被选入，精度下降

---

### Q：检索后有没有 Rerank？

**当前实现：** RRF 融合后直接送 LLM，无 Rerank。

**Trade-off：**
- **不 Rerank**：延迟低，但语义匹配深度有限，Top-5 中可能混入噪声
- **Cross-encoder Rerank**：每个 chunk 再过一次重排模型，精度更高，但多一次模型推理的额外延迟

---

## 九、RAG 链路

### Q：Top-K 取多少？如果检索结果里根本没有答案怎么办？

**A：**
- K = 5
- 防幻觉：System Prompt 中要求"优先基于检索资料回答"，但是**软约束**，没有硬性拒答逻辑
- 生产上可以补：当 Top-5 的相似度分数均低于阈值时，引导 LLM 回复"当前知识库中未找到相关内容"

---

### Q：多轮对话中如何处理指代消解？

**当前实现（已知缺陷）：**
- 只拿当前轮的原始 query 做混合检索
- 历史消息拼接在检索结果之后一起送 LLM
- **问题**：多轮指代（"那第二点呢？"）导致检索 query 语义残缺，召回质量差

**正确做法 — Query Rewriting：**

```
最近 N 轮对话 + 当前问题
    ↓
LLM 改写 → 独立可检索的完整问题
    ↓
用改写后的问题做混合检索
    ↓
生成阶段仍保留原始历史消息（LLM 可利用长上下文）
```

**为什么不把历史对话塞进检索 query：**
- 检索需要"短、精确、聚焦的语义"，历史对话是噪声
- 历史塞进去会导致 BM25 词项污染（无关词汇干扰 TF-IDF）、向量语义漂移
- 全量历史还可能超过 context window，token 消耗线性增长

---

## 十、亮点深挖概括（面试口语版）

### 核心设计决策

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 分片大小 | 固定 5MB | MinIO 推荐范围，知识库文档场景分片数少 |
| 秒传粒度 | 按用户隔离 | 避免跨用户越权，尤其删除级联问题 |
| 合并方式 | MinIO ComposeObject | 服务端合并，数据不过业务机，保持上传-处理分离 |
| 分片续传 | 应用层（非 MinIO Multipart） | 状态完全自控，无 UploadID 生命周期问题 |
| 幂等保障 | UPDATE WHERE + affected rows | 原子 CAS，无竞态 |
| 切分策略 | 递归文本切分 + 10% overlap | 自然边界优先，减少语义截断 |
| 向量检索 | HNSW kNN（ES dense_vector） | 近似检索，避免全量扫描 |
| 检索融合 | RRF（dense + BM25） | 无需调参，稳定 |
| 中文分词 | IK（索引 max_word / 查询 smart） | 避免 BM25 退化为字符匹配 |

### 已知缺口（可主动提出）

1. **Kafka 可靠投递**：当前无 Outbox，合并后 Kafka 失败是静默丢失；改进方向是 Outbox Pattern 或 pending 状态 + 定时补偿
2. **Pipeline DLQ**：重试耗尽只改状态，无死信队列和告警
3. **孤儿分片**：清理失败只打日志，需补 MinIO ILM 前缀过期规则
4. **外部 API 限流**：embedding 接口无指数退避和熔断
5. **多轮指代**：当前未做 Query Rewriting，多轮对话召回质量有损
6. **MinIO 对象 key 隔离**：`merged/{fileName}` 未带 userID，存在跨用户同名覆盖风险
7. **无系统化 RAG 评估**：只做功能性验证，缺 Recall@K / MRR 离线评测集
