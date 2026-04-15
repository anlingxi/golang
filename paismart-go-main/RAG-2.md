# RAG 亮点二：混合检索 + RRF 融合排序 — 面试深挖整理

> 亮点原文：针对单路检索语义漏召回与关键词漏匹配问题，设计稠密向量 + BM25 并行检索；Go 侧手写 RRF 融合算法，以排名代替原始分数解决多路量纲不统一问题，兼顾语义理解、术语精确匹配与关键词覆盖。

---

## 目录

1. [面试问答汇总（Q1–Q28）](#面试问答汇总)
2. [正式面试框架 & 深度递进关系](#正式面试框架)

---

## 面试问答汇总

---

### Q1：什么情况下稠密向量检索会漏召回？什么情况下 BM25 会漏匹配？

**A：**
- **向量漏召回**：含版本号、精确型号等精确匹配需求时。例如用户问「MySQL 8.0.36 驱动配置」，向量检索可能召回大量 MySQL 8.0 配置/数据库驱动等相似内容，却把版本号完全匹配的那条漏掉。
- **BM25 漏匹配**：词面表达和知识库存储术语不同时。例如用户问「显存问题」，知识库存的是「GPU 内存」，BM25 词项无法对齐，导致漏匹配或排名靠后。

---

### Q2：两路检索是真正并发执行的吗？如何实现？

**A：** 使用 `errgroup.WithContext`，一条直接跑 BM25，另一条先生成 query embedding 再跑 KNN，两者并发执行。

> **💡 代码细节（来自 gpt2.md）：**
>
> 当前代码改造前是串行的（先 embedding → kNN → BM25）。改造后在 `search_service.go:100` 用 `errgroup.WithContext(ctx)` 起两个并发任务：
> - 一条直接跑 BM25（不依赖 embedding，可立即发起）
> - 另一条先 `CreateEmbedding`，再跑 kNN
>
> 这样总耗时从 `embedding + knn + bm25` 三段串行，压缩到 `max(bm25, embedding + knn)` 两条并行链路中较慢的一条。
>
> 更进一步的优化：BM25 不依赖 embedding，所以可以让 BM25 和 embedding 生成同时并发，kNN 等 embedding 完成后立刻发起。

---

### Q3：RRF 公式是什么？k 值取多少，为什么？

**A：**

$$\text{score}(d) = \sum_{r \in R} \frac{1}{k + \text{rank}_r(d)}$$

k 取 60，参考官方推荐值。

> **💡 细节展开：**
>
> k=60 是 Elasticsearch 和学术论文中最常见的默认推荐值。k 的作用是平滑排名差异——避免排名第 1 的文档分数过于突出，使得其他靠前文档的分数差距更均匀。k 越小，排名靠前的文档优势越大；k 越大，排名之间差异越平滑。60 在大多数场景下是稳定的默认值，无需针对数据集调参。

---

### Q4：embedding 服务超时/报错，整个请求怎么处理？

**A：** 直接返回错误，不做 BM25-only 降级。考虑是：混合检索的设计目标是双路融合，如果 embedding 挂了只返回 BM25，相关性和召回稳定性都会下降，不如明确返回错误让调用方知道检索未按预期完成。

> **💡 代码细节（来自 gpt2.md）：**
>
> 有两个入口需要区分：
> - **搜索接口**：`search_handler.go:49` 调 `HybridSearch`，embedding 失败 → `g.Wait()` 返回 error → 直接 500
> - **聊天场景**：`chat_service.go:117` 调检索，失败后 `chat_service.go:118` 只记 warning，继续走「无检索上下文模式」，不会让整个对话失败
>
> 所以搜索接口更脆，聊天侧有兜底。但 embedding client 本身没有配独立超时，主要依赖 request context（`c.Request.Context()`），没有专门的 embedding 超时配置。

---

### Q5：errgroup.WithContext 如何传递取消信号？内部调用是否正确监听 ctx？

**A：** `errgroup.WithContext` 返回派生 ctx，任意 goroutine 返回 error 后该 ctx 被 cancel。内部调用都把 `gctx` 传下去了——BM25 的 ES 请求用 `Search.WithContext(ctx)`，embedding 调用用 `http.NewRequestWithContext(ctx)`，kNN 的 ES 请求也走 `WithContext(ctx)`，所以各阶段 I/O 均能感知取消并尽快返回。

> **💡 细节展开：**
>
> `errgroup` 的取消不会"强制停止"另一个 goroutine 的 Go 代码执行，而是通过 ctx 通知阻塞中的 I/O 操作返回。只要 HTTP 请求和 ES 请求都绑定了 ctx，任意一路失败后，另一路在等待 I/O 响应时能感知到 ctx 取消并提前返回，不会无限阻塞。这里的关键前提是：没有长时间的 CPU-bound 循环，全是网络 I/O，所以 ctx 传播能有效发挥作用。

---

### Q6：只出现在一路的文档，RRF 如何处理？排名是多少？

**A：** 只出现在一路的文档不会被丢掉，另一路贡献记 0。它的 RRF 分数就是单路的 `1/(k + rank)`。如果排名很靠前，仍然可能超过两路都出现但排名都靠后的文档——RRF 看的是 rank，不是原始 score。

> **💡 细节展开：**
>
> 举例：单路文档排第 1，分数 = 1/(60+1) ≈ 0.0164。
> 双路文档分别排第 50，分数 = 1/(60+50) + 1/(60+50) ≈ 0.0182。
> 双路文档分别排第 1，分数 = 1/61 + 1/61 ≈ 0.0328。
>
> 所以双路命中通常更占优，但单路排名极高时也能超过双路但排名靠后的文档。

---

### Q7：Go 侧 RRF 核心数据结构是什么？

**A：** `map[docID]*rrfEntry`，包含累积 RRF 分数和最佳排名。双路命中时，同一 doc 直接累加分数。最后 dump 成 slice，用 `sort.Slice` 按 RRFScore 降序排；分数相同按 BestRank 更小的排前面做 tie-break。

> **💡 代码细节（来自 gpt2.md）：**
>
> key 的生成逻辑在 `search_service.go:378` 的 `hitKey`：
> - 优先用 `VectorID`（在 `es_document.go:19` 注释里写了，通常是 `fileMd5 + chunkId`）
> - 没有则用 ES `_id`
> - 再没有则退化成 `fileMd5#chunkId`
>
> `fusedEntry` 结构（`search_service.go:319`）含三个字段：`Hit`（原始命中内容）、`RRFScore`（累计分数）、`BestRank`（tie-break 用的最好排名）。
>
> 排序代码在 `search_service.go:355–366`：先收集 map values 到 slice，再 `sort.Slice`。

---

### Q8：top-K 怎么定的？有没有做质量评估？

**A：** K=5，当前没有做过质量评估。更合理的做法是离线测不同 K 值看答案正确率、引用命中率、召回覆盖率，同时看线上用户追问比例。K 不是越大越好，过大会把弱相关 chunk 塞进来，稀释模型注意力。

> **💡 代码细节（来自 gpt2.md）：**
>
> 仓库里有两个 topK：
> - 对外搜索接口：`search_handler.go:35` 从请求参数读，默认 10
> - 聊天场景：`chat_service.go:117` 写死 `HybridSearch(..., 5, user)`
>
> 关键细节：召回阶段不是每路只取 5，而是先放大召回池。`search_service.go:26` 的 `recallMultiplier = 30`，所以每路先取 `topK * 30`（即 150 条），RRF 后截断到 top-5。这是为了避免 RRF 融合时候选集太小导致好文档被丢掉。

---

### Q9：没有 retry 策略，embedding 抖动时风险可以接受吗？

**A：** 短期可以接受，因为问答侧对检索失败有降级处理（继续回答，只是基于模型本身能力）。但对纯搜索接口，没有 retry 让用户直接看到失败体验不理想。更好的做法是对瞬时错误（timeout、5xx、429）重试 1-2 次，用带 jitter 的指数退避，并受 request context deadline 控制。

> **💡 细节展开：**
>
> 不应无脑重试——重试策略应只针对瞬时错误，不针对 4xx 业务错误。退避建议：100ms → 300ms，或 200ms → 500ms，加 ±50ms 的 jitter 防止惊群。整体重试总耗时必须受 ctx deadline 约束，避免把整条搜索链路拖延。当前 embedding client 有 `http.Client.Timeout = 10s`，但没有 retry 逻辑。

---

### Q10：docID 是什么粒度？有没有做 document-level 折叠？

**A：** docID 是 chunk 粒度，本质是 `fileMD5 + chunkID`（无则用 ES _id）。没有做 document-level 折叠，同一文档的不同 chunk 作为独立结果参与 RRF。

> **💡 代码细节（来自 gpt2.md）：**
>
> `VectorID` 在 `es_document.go:19` 注释明确：通常是 `fileMd5 + chunkId`。
> 返回 DTO 里带的是 `ChunkID`（`es_document.go:21` 和 `search_service.go:187`）。
> 给 LLM 组上下文时也是把每条结果的 `TextContent` 直接拼进去（`chat_service.go:181`），没有先按文档聚合。

---

### Q11：map 无序，如何得到最终排序列表？

**A：** 把 map dump 成 slice，用 `sort.Slice` 排序。主键是 RRFScore 降序，分数相同按 BestRank 更小的排前面。

> **💡 代码位置：** `search_service.go:355–366`

---

### Q12：K=5 够吗？如何判断是否合适？有没有考虑动态 K？

**A：** 5 条对单事实问题通常够，对需要综合多段知识的问题不一定够。判断是否合适需要离线评测（不同 K 对答案正确率、引用命中率、召回覆盖率的影响）和线上指标（用户追问比例等）。有考虑过动态 K——事实/定义型用小 K，对比/流程/总结型适当增大，可以根据用户意图分类来判断。

---

### Q13：chunk 无折叠导致多样性压缩的问题，如何修？

**A：** 意识到了这个问题。修法分两层：
1. **最简单**：per-document cap，同一文档最多保留 1-2 个 chunk，先把多样性拉回来
2. **进一步**：做 document-level collapse + MMR 重排，对已选 chunk 之间过于相似的内容进行惩罚，减少同文档同主题重复片段挤占上下文

> **💡 细节展开：**
>
> 这个问题的本质是：top-K 里来自同一文档的多个 chunk，会让 LLM 看到的有效「知识来源」篇数降低，影响跨文档综合能力，同时浪费 token（语义相近内容边际信息增益很低）。
>
> 优先级：先做 per-document cap（改动小，收益直接），再做 MMR 或 document-level collapse（效果更好，但实现复杂度更高）。

---

### Q14：KNN 用的是 ES 原生还是 script_score？两者区别？

**A：** 用的是 ES 原生 KNN（基于 HNSW 的近似检索）。和 `script_score` 的区别：
- 原生 KNN：基于图结构，不遍历所有向量，速度快但可能漏掉极少量真正相近的向量（近似检索）
- `script_score`：暴力全量扫描，精确度最高，但文档量大时延迟显著升高

---

### Q15：BM25 那路，query 做了什么预处理？

**A：** 做了轻量预处理，去掉口语化表达（「请问」「是什么」等）和特殊字符。

> **💡 细节展开：**
>
> 这是较轻量的预处理，未涉及 query 改写、query 扩展、同义词补全等。更完整的 BM25 预处理还可以包括：
> - 停用词过滤（防止无意义词项污染 TF-IDF）
> - query 扩展（加同义词、专业别名）
> - 中文分词一致性检查（确保查询分词模式和索引分词模式匹配）

---

### Q16：有没有做 rerank？RRF 分数能代表最终相关性吗？

**A：** 没有做 rerank。RRF 分数不能完全代表最终相关性排名——它只对排名做融合，没有对 chunk 语义进行感知。两个 RRF 分数相近的文档，rerank 后可能相关性差距很大。加上 rerank 预期能明显提升答案质量。

> **💡 细节展开：**
>
> RRF 是无监督的融合方法，它的优势是稳定、不需要调参；但它本质上只看「排名位置」，不看 query 和 chunk 的语义匹配深度。
>
> Cross-encoder rerank 会把 query 和每个 chunk 拼接后做联合编码，可以捕获精细语义关系，精度更高。代价是每个候选都需要一次单独推理，不适合大候选集，通常放在 RRF 之后对 top-20 做精排。

---

### Q17：MMR 公式是什么？λ 怎么设？

**A：**

$$\text{MMR}(d) = \lambda \cdot \text{sim}(d, q) - (1-\lambda) \cdot \max_{s \in S} \text{sim}(d, s)$$

λ 控制相关性和多样性的 tradeoff：
- λ 调小 → 多样性更重要（适合推荐、探索型场景）
- λ 调大 → 相关性更重要（适合精确专业问答）
- 可从 0.5 开始，根据实际效果调整

---

### Q18：HNSW 参数（m、ef_construction、num_candidates）怎么配的？

**A：** 当前使用 ES 默认值，未针对业务数据调优。
- `m`：每个节点的邻居数，越大精度越高，但内存占用和构建时间越大
- `ef_construction`：建图时候选集大小，越大索引质量越高但构建越慢
- `num_candidates`：查询时候选池大小，设置为 30*topK，越大召回率越高但查询延迟也高

> **💡 细节展开：**
>
> `num_candidates` 的调优经验：通常建议设为目标 K 的 10 倍。过大 → 扫描节点多，延迟升高；过小 → 候选池不足，真正相似的文档未被选入，精度下降。
>
> `m` 默认值通常是 16，`ef_construction` 默认 100。对召回精度要求高的场景可以调高，但需要评估内存和构建时间的增长。

---

### Q19：了解 HyDE（假设文档嵌入）吗？

**A：** 了解。核心思路是：用 LLM 先生成一段假设答案，再用这段答案做 embedding 检索——因为「答案和答案更匹配」，向量空间上更接近目标文档。

副作用：
1. 多一次 LLM 调用，增加成本和时延
2. 更严重的问题：如果知识超出 LLM 本身的知识储备，生成的是幻觉答案，用幻觉去检索等于「幻觉 + 乱查」，效果可能更差

> **💡 细节展开：**
>
> HyDE 在通用知识问答上效果不错，但在垂直领域（专有文档、内部知识库）中风险较高，因为 LLM 对这些领域没有先验知识，生成的假设答案质量低。
>
> 更保守的替代方案：query rewrite（基于历史上下文改写）、query expansion（加同义词/专业别名）、step-back prompting（先问更抽象的问题再检索）。

---

### Q20：cross-encoder vs bi-encoder 本质区别是什么？

**A：** 本质区别是**是否有跨文本的 attention**。
- **bi-encoder**：query 和 document 分别独立编码，各自压缩成一个向量，用点积/余弦相似度。document embedding 可以提前离线计算，查询时只需计算 query embedding，适合大规模召回。
- **cross-encoder**：把 query 和 candidate chunk 拼接后联合编码，两者之间有完整 attention 交互，精度更高。但每个候选都需要单独推理，无法预计算，不适合召回阶段大量使用，通常用于 top-K 精排。

---

### Q21：多轮对话中指代消解怎么处理？

**A：** 当前没有处理，只是简单把历史会话 + 检索结果一起丢给 LLM，没有做 query rewrite。多轮指代（「那第二点呢？」）导致检索 query 语义残缺，召回质量有损。

> **💡 细节展开（query rewrite 设计方案）：**
>
> 正确做法是在 `chat_service.go:117` 调 `HybridSearch` 之前，加一个 `QueryRewriteService`：
>
> ```
> history + current query → LLM rewrite → standalone query → HybridSearch → RRF → topK → LLM 生成
> ```
>
> - 输入：当前轮问题 + 最近几轮对话历史 + 必要的实体/时间信息
> - 输出：适合检索的 standalone query（去掉代词、补全主语）
> - 实现：先用规则层处理明显代词消解，复杂场景再用轻量 LLM 改写
> - 原始问题继续给生成模型，rewrite 只服务检索
> - 降级：rewrite 失败/低置信度时，直接用原始 query
>
> 引入的新问题：语义漂移、延迟增加、错误放大、评估链路更复杂。

---

### Q22：chunk 切分策略是什么？有没有 overlap？

**A：** 递归文本切分，优先按自然边界（段落、换行、句号）切，边界不够才退化到字符级。有做 overlap，取 chunk 大小的 10%。chunk 太大混入多个主题导致语义稀释，太小语义不完整且上下文碎片化。10% overlap 在当前场景够用，过大会导致重复严重。

---

### Q23：端到端 P99 延迟大概多少？瓶颈在哪里？

**A：** 没有做端到端延迟测评。瓶颈分析：
- **检索链路**：主要在外部 I/O——embedding 服务调用（跨网络外部 API）和 ES 请求（BM25 + kNN），Go 侧 RRF 计算不是瓶颈
- **聊天端到端**：最大瓶颈是 LLM 生成，尤其是首 token 和完整输出时间

> **💡 代码细节（来自 gpt2.md）：**
>
> 仓库里有请求日志延迟埋点（`middleware/logging.go:42` 记录 HTTP latency）和 callback duration 日志（`eino/callbacks/logging_handler.go:53` 打印阶段耗时），但这是日志口径，不是聚合 P99 指标。
>
> 经验量级参考：检索链路百毫秒到一两秒（取决于 embedding API 和 ES 响应），聊天端到端通常秒级（P99 由 LLM 生成拉高）。更标准的做法是按阶段打点：embedding、BM25、kNN、RRF、prompt build、first token、full completion 分开看。

---

### Q24：如何设计多轮对话下的 query rewrite 模块？

**A：** 设计为检索前置模块，放在 `chat_service.go:117` 调 `HybridSearch` 之前。
- **输入**：当前用户问题 + 最近几轮对话历史 + 用户/租户信息
- **输出**：结构化结果（rewritten_query、intent、entities、confidence、need_clarification）
- **实现**：规则层（代词消解、时间词归一化）+ 轻量 LLM（复杂改写）
- **风险控制**：低置信度时回退原始 query，不阻塞主链路
- **引入的新问题**：语义漂移、过度补全、延迟增加、错误放大

---

### Q25：了解 ColBERT 吗？和 bi-encoder 的本质区别？

**A：** 本质区别是粒度——bi-encoder 把所有 token 压缩进一个向量，ColBERT 给每个 token 保留一个向量。查询时用 MaxSim 操作：对 query 的每个 token，找它和 document 所有 token 向量中最相似的，加总得分。这样保留了更多语义信息（同一个 token 在不同语境向量不同）。

没有普及的原因：存储成本过高（每个 token 一个向量，文档向量数量爆炸），且带来的收益相对于成本并不是跨量级提升。

> **💡 纠错说明：**
>
> ColBERT 是 **multi-vector dense** 模型，不是稀疏向量。稀疏向量（SPLADE 等）是另一条路：学出来的高维稀疏权重，更接近可解释的 term weight，和 ColBERT 完全不同。
>
> 对比三类方案：
> - **Bi-encoder**：1 个 dense vector per doc，查询时点积/余弦
> - **ColBERT**：多个 dense vectors per doc（每 token 一个），MaxSim 操作
> - **SPLADE**：高维稀疏 vector，非零位对应 term weight，类 BM25 但学习得来

---

### Q26 & Q27：embedding 领域微调怎么做？

**A：**

**数据准备**（最关键）：
- 真实搜索日志（点击/停留作为弱监督正样本）
- 知识库标题 + chunk 天然配对
- 人工标注样本
- LLM 从领域文档反向生成 query

**样本构造**：query-positive-negative 三元组，重点做 hard negative（BM25 或旧 embedding 检出的假阳性，训练价值最高）

**训练目标**：对比学习（InfoNCE、Multiple Negatives Ranking Loss、Triplet Loss），不是普通分类

**模型选型**：可训练的开源 embedding 基座（BGE、E5、gte），用 LoRA 或继续训练做领域适配

**评估**：Recall@K、MRR、NDCG 以及端到端问答效果，不能只看 loss

**现实判断**：如果当前用的是不支持 fine-tune 的商用 embedding API，这条路不现实，优先考虑开源模型替换，或先上 reranker + query rewrite（通常比直接谈 embedding 微调更落地）

---

### Q28：上线到日活 10 万的 B 端 SaaS，3 个最大工程风险？

**优先级：安全隔离 > 可用性/P99 > 检索效果**

**P0 — 多租户数据隔离和权限泄漏**
- 当前检索权限主要靠查询时 `user_id / is_public / org_tag` 过滤（`search_service.go:208/223`）
- 风险：过滤条件写错、权限状态不一致、prompt 组装绕过校验，可能把别的租户内容带给模型
- 缓解：defense in depth（检索前过滤 + 入模前二次鉴权）；索引层按租户 alias 或分索引隔离；权限变更做增量重建；自动化越权测试 + 审计日志

**P1 — 外部依赖尾延迟和可用性**
- 在线链路有实时 embedding、BM25、kNN、LLM，瓶颈全在外部 I/O
- embedding 失败搜索接口直接 500（`search_service.go:100`）；聊天侧才有降级（`chat_service.go:117`）
- 10 万 DAU 下真正打垮系统的是依赖抖动放大的 P99
- 缓解：阶段级指标和 SLO；有限 retry + jitter backoff + circuit breaker；搜索接口支持受控降级（embedding 挂了走 BM25-only）；热 query 结果缓存

**P2 — 检索质量稳定性和上下文利用率**
- 当前 chunk 粒度 RRF + 固定 top-5 给模型（`chat_service.go:117`），无 document-level 折叠
- 同一文档多 chunk 可能挤占 top-K，有效知识来源篇数被压缩
- 缓解（按成本递进）：per-document cap → document-level collapse/MMR → reranker → query rewrite + 动态 K → 离线评测集 + 线上 AB

---

## 正式面试框架

### 问题深度递进关系

面试官通常按以下 6 个层次从浅到深拷打，每层都可能拦截候选人：

```
第一层：概念理解（是什么，为什么）
    ↓ 答出来 → 进入下一层
第二层：实现细节（怎么做的，用了什么）
    ↓
第三层：边界情况和局限（如果 X 怎么办，没做 Y 有什么问题）
    ↓
第四层：已知缺陷的改进方向（你会怎么修，怎么优化）
    ↓
第五层：你没做但应该了解的技术（知识广度考察）
    ↓
第六层：生产化 & 系统设计（上规模后的工程风险）
```

---

### 混合检索 + RRF 的标准面试递进路径

#### 第一层：概念理解

| 问题 | 考察点 |
|------|--------|
| 为什么要做混合检索？单路有什么问题？ | 理解 dense/sparse 各自的局限 |
| RRF 是什么？公式是什么？为什么选 RRF 而不是加权求和？ | 知道 RRF 的动机和原理 |
| BM25 和 embedding 检索分别适合什么场景？ | 能举具体例子 |

#### 第二层：实现细节

| 问题 | 考察点 |
|------|--------|
| 两路检索是并发的吗？怎么实现？ | goroutine/errgroup 使用 |
| RRF 的 Go 侧数据结构是什么？ | 代码设计能力 |
| docID 是文档粒度还是 chunk 粒度？ | 理解自己的实现 |
| ES KNN 用的是原生 kNN 还是 script_score？HNSW 参数怎么配？ | ES 使用深度 |

#### 第三层：边界情况

| 问题 | 考察点 |
|------|--------|
| 只出现在一路的文档，RRF 怎么处理？ | 边界理解 |
| embedding 服务挂了，会怎样？有没有降级？ | 故障处理意识 |
| 一篇文档切了很多 chunk，RRF 后可能同一文档多 chunk 占满 top-K，怎么看？ | 召回多样性问题 |
| 多轮对话里指代词怎么处理？ | 多轮场景覆盖 |

#### 第四层：改进方向

| 问题 | 考察点 |
|------|--------|
| 如果要修 chunk 折叠问题，你会怎么做？ | MMR、per-doc cap |
| 如果加 rerank，选 cross-encoder 还是 bi-encoder？ | 选型判断 |
| K 值怎么动态调整？ | 工程优化意识 |
| 如何设计 query rewrite 模块？ | 系统设计能力 |

#### 第五层：知识广度

| 问题 | 考察点 |
|------|--------|
| 了解 HyDE 吗？它的副作用是什么？ | 前沿方案了解 |
| ColBERT 和 bi-encoder 的本质区别？ | embedding 原理深度 |
| embedding 领域微调怎么做？用什么训练目标？ | ML 工程能力 |

#### 第六层：生产化

| 问题 | 考察点 |
|------|--------|
| P99 延迟多少？瓶颈在哪？ | 性能意识 |
| 日活 10 万 B 端 SaaS，最大的 3 个工程风险是什么？ | 系统设计、优先级判断 |
| 多租户隔离怎么做？ | 安全意识 |

---

### 面试官拦截点总结

以下是最常见的「坑」，需要主动避开或主动说出已知缺陷：

| 风险点 | 标准应对 |
|--------|---------|
| 说「并发检索」但实际是串行 | 主动说明改前是串行，改后是 errgroup 并发，说清楚改动原因 |
| 说「断点续传」被质疑不算 | 说「应用层分片续传」，解释和 MinIO 原生 Multipart 的区别 |
| 被问 document-level 折叠 | 直接承认没做，说清楚问题和两层修法 |
| 被问 rerank | 承认没做，说清楚 RRF 的局限和加 rerank 的预期收益 |
| 被问 ColBERT | 不要把它和稀疏向量混淆，ColBERT 是 multi-vector dense |
| 被问 P99 | 不要乱报数字，说链路分析和打点思路 |
| 被问 embedding 微调 | 不要只说「微调一下」，要说数据、目标函数、评估指标 |
| B 端风险优先级 | 安全隔离 > 可用性 > 效果，不要说「效果最重要」 |

---

### 面试中主动加分的话术

1. **主动暴露已知缺陷**：「我们当前没有做 document-level 折叠，这是个已知问题，我知道怎么修……」
2. **代码可引用**：能说出具体代码位置（如 `search_service.go:100`），显示真实实现经验
3. **tradeoff 意识**：「不是效果越好越好，还要考虑时延、成本和工程复杂度……」
4. **优先级判断**：「B 端 SaaS 最先不能出的是串租户，其次是可用性……」
5. **知道自己的边界**：「当前还没有做评测，这个 K 值更像经验值而不是实验值……」
