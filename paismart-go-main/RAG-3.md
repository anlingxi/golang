# RAG 亮点四：Kafka 消费可靠性 — 面试深挖整理

> 亮点原文：实现手动 offset 提交与基于 Redis 的重试计数（最多3次），结合 MySQL 文档状态机（pending → processing → completed）与 SELECT FOR UPDATE 保证消费幂等性，防止 rebalance 导致重复处理。

---

## 目录

1. [面试问答汇总（Q1–Q8）](#面试问答汇总)
2. [正式面试框架 & 深度递进关系](#正式面试框架)

---

## 面试问答汇总

---

### Q1：自动提交 vs 手动提交——你为什么选手动？

**面试回答：**

Kafka 自动提交存在消息丢失问题：消费者拿到消息还没处理完，自动提交先提交了 offset，此时消费者崩溃，消息被标记为已消费但实际没处理，等于丢失。手动提交确保消息处理完成后才 commit offset，不会出现"提交了但没处理"的情况。

> **💡 代码细节：**
>
> 实现上使用 `segmentio/kafka-go` 的 `FetchMessage` + `CommitMessages` 组合，而不是 `ReadMessage`（后者自动提交）。
>
> 见 `pkg/kafka/client.go:62`：
> ```go
> m, err := r.FetchMessage(context.Background())
> // ... 处理 ...
> if err := r.CommitMessages(context.Background(), m); err != nil { ... }
> ```
>
> 只有在 `processor.Process` 成功后才调用 `CommitMessages`；失败时根据重试次数决定是否提交。

---

### Q2：`FetchMessage` 在不提交 offset 时，下一次调用返回什么？重试怎么触发？

**面试回答：**

`FetchMessage` 每次调用会取下一条未取过的消息，不管上一条有没有 commit。不提交 offset，下一次 `FetchMessage` 拿到的是下一条新消息，不是同一条。

真正的重试发生在：消费者重启、reader 重建、或 group rebalance 之后——因为此时 Kafka 会从上一次已提交的 offset 重新发送未提交的消息。

> **💡 代码细节与修正：**
>
> 原始代码（已修改）中存在一个理解偏差：`attempts < 3` 时不 commit，认为 Kafka 会自动重投同一条——但 `FetchMessage` 并不会在当前会话内重投，它会直接推进到下一条消息。
>
> 修正后的实现：在处理失败时，在消费者内部对当前消息做重试循环（加指数退避），而不是靠 Kafka 层面的重投来实现"3次重试"语义。
>
> 这两种方式的区别：
> - Kafka 重投：等待 rebalance/重启，延迟不可控，最多 3 次语义依赖消费者重启次数
> - 内部重试循环：当次处理内可控，可以加 backoff，失败 3 次后 commit offset 终止

---

### Q3：retry key 用 FileMD5 的 bug

**面试回答：**

retry key 用 FileMD5 存在碰撞问题：同一个文件被同一个用户上传两次，两条 Kafka 消息共享同一个 retry key，第一条消息失败了 2 次，第二条消息第一次失败后计数变 3，被直接跳过——但这条消息按设计还有两次重试机会。

正确做法是用 Kafka 消息的 **partition + offset** 作为 retry key，这是一条消息在 Kafka 中全局唯一的标识。

> **💡 代码细节（原始代码的问题）：**
>
> 见 `pkg/kafka/client.go:86`：
> ```go
> attemptsKey := fmt.Sprintf("kafka:attempts:%s", task.FileMD5)
> ```
>
> FileMD5 是文件内容的哈希，代表文件内容，不代表一次具体的上传事件。同内容文件的多次上传会共享这个 key。
>
> 修正方案：
> ```go
> attemptsKey := fmt.Sprintf("kafka:attempts:%d:%d", m.Partition, m.Offset)
> ```
>
> `m.Partition` 和 `m.Offset` 合起来在一个 topic 内唯一标识一条消息，不会跨消息碰撞。

---

### Q4：SELECT FOR UPDATE + 状态机——完整流程

**面试回答：**

状态机逻辑不在 `kafka/client.go`，而在文档处理服务层（`TryMarkFileProcessing`）。

完整流程：

1. 消费者取到消息，调用 `processor.Process`
2. Process 第一步：开短事务，对 `file_upload` 表按 `(file_md5, user_id)` 执行 `SELECT FOR UPDATE`，锁住该行
3. 拿到锁后检查 `process_status`：
   - 是 `pending` 或 `failed`：在同一事务内更新为 `processing`，提交事务
   - 是 `processing` 或 `completed`：说明已被抢占或完成，跳过，return
4. 事务提交后，开始真正的文件处理（MinIO 读文件、解析、切片、embedding、写 ES）
5. 处理成功：更新状态为 `completed`
6. 处理失败：更新状态为 `failed`

SELECT FOR UPDATE 防的是：rebalance 后两个消费者同时拿到同一条未提交 offset 的消息，都要处理同一个文件——其中一个会先拿到行锁并把状态改为 `processing`，另一个等到锁释放后看到状态不是 `pending`/`failed`，直接跳过。

> **💡 代码细节：**
>
> 短事务（只锁"检查 + 改状态"这一小段）是关键设计决策：如果整个文件处理（可能几秒到几分钟）都包在事务里，行锁会持有很久，阻塞其他需要读这行的操作。
>
> 拆分为：
> - 短事务（毫秒级）：SELECT FOR UPDATE → 改 processing → 提交
> - 长操作（秒级）：文件处理，不持有数据库锁
>
> `(file_md5, user_id)` 上需要有联合索引，否则 SELECT FOR UPDATE 会退化为表锁。

---

### Q5a：processing 状态卡死

**面试回答：**

这是当前实现的一个真实缺口：消费者把状态写成 `processing` 之后崩溃，状态永久卡住。消费者重启后重新消费该消息，看到状态是 `processing`，判断"有人在处理"，直接跳过。文件永远不会被处理。

正确的修法是给 `processing` 状态增加 **lease / heartbeat 机制**：

- 每次把状态改为 `processing` 时，同时写入 `processing_started_at` 时间戳
- 后台定时任务扫描：`process_status = 'processing' AND processing_started_at < NOW() - interval`
- 超时未完成的记录回滚到 `pending`，允许重新被消费

> **💡 代码细节：**
>
> 超时阈值设置需要大于最长正常处理时间（比如大文件解析可能需要 5 分钟，阈值就不能设 1 分钟）。
>
> 也可以在消费者处理期间每隔一段时间更新 `processing_heartbeat_at`，后台任务判断 heartbeat 超时而不是 started_at 超时，这样处理中的任务不会被误回滚。

---

### Q5b：事务范围

**面试回答：**

短事务在"检查状态 + 改 processing"后立刻提交，然后才开始实际的文件处理（MinIO 读取、解析、embedding、写 ES）。

两种极端选择的问题：
- **整个处理包在事务里**：行锁持有几分钟，阻塞其他读写；事务过大，数据库连接长期占用
- **完全不用事务**：SELECT FOR UPDATE 要有事务才能起锁的作用，否则锁不住

> **💡 代码细节：**
>
> `TryMarkFileProcessing` 是一个独立的短事务方法，只负责"抢锁 + 改状态"，提交后返回 bool 表示是否抢到处理权。
>
> 文件处理本身不持有数据库事务，失败时通过独立的 `UpdateStatus(failed)` 更新状态。

---

### Q5c：rebalance 场景下的并发时序

**面试回答：**

具体时序：

1. 消费者 A 取到消息，`TryMarkFileProcessing` 成功，状态变 `processing`，开始处理文件
2. 此时 rebalance 发生，Kafka 把这个 partition 重新分配给消费者 B
3. 消费者 B 从上次提交的 offset（消费者 A 还没提交）重新消费，拿到同一条消息
4. B 执行 `TryMarkFileProcessing`，`SELECT FOR UPDATE` 锁行，读到状态是 `processing`
5. B 判断"有人在处理"，直接跳过，不重复处理
6. 消费者 A 如果正常完成，写 `completed`，commit offset；B 下一轮不再看到这条消息

> **💡 代码细节：**
>
> 这个场景的正确性依赖一个前提：消费者 A 没有崩溃，只是 rebalance 导致 partition 转移。如果 A 崩溃了，状态卡在 `processing`，B 会跳过，导致 Q5a 的卡死问题。
>
> 所以 SELECT FOR UPDATE 解决了"并发正常消费的幂等性"，但没有解决"处理中崩溃后的恢复"——这两个问题需要分开处理。

---

### Q6：两套机制的关系（Redis 计数 vs MySQL 状态机）

**面试回答：**

Redis 计数已删除，当前只保留 MySQL 状态机。

两套并存时的逻辑混乱：
- Redis 计数控制"这条消息还允不允许重试"
- MySQL 状态机控制"这条文件处理请求能不能被处理"

两者逻辑不串联：Redis 说"可以重试"，MySQL 说"已经 completed"，到底听谁的？如果 Redis 先到 3 次 commit 了 offset，MySQL 状态还是 failed，这条文件就被永久跳过了。

现在只用 MySQL 状态机：Kafka 决定消息会不会重投，MySQL 状态机决定重投后能不能抢到处理权。逻辑更清晰，单一职责。

> **💡 代码细节：**
>
> 删除 Redis 计数后，失败重试的上限由 Kafka 消费者的重试配置或外部告警机制控制，不在应用层硬编码 3 次。
>
> 如果要保留"最多重试 N 次"的语义，可以在 `file_upload` 表加 `retry_count` 字段，每次 `failed` 回滚时递增，超过阈值后标记为 `permanently_failed`，不再允许重入 `pending`。

---

### Q7：消费者的死亡——break on error

**面试回答：**

原始代码 `FetchMessage` 出错直接 `break`，消费者 goroutine 永久退出，没有自动恢复，一次网络抖动就能杀死消费者直到进程重启。

正确写法是 supervisor 模式：

```go
// 外层：监督者，负责重建 reader 和退避
func StartConsumer(cfg config.KafkaConfig, processor TaskProcessor) {
    backoff := time.Second
    for {
        err := consumeLoop(cfg, processor)
        if isFatalError(err) {
            log.Fatalf("不可恢复错误，退出: %v", err)
            return
        }
        log.Warnf("消费者退出，%v 后重试: %v", backoff, err)
        time.Sleep(backoff)
        backoff = min(backoff*2, 30*time.Second) // 指数退避，上限 30s
    }
}

// 内层：消费循环，只负责 Fetch -> 处理 -> Commit
func consumeLoop(cfg config.KafkaConfig, processor TaskProcessor) error {
    r := kafka.NewReader(...)
    defer r.Close()
    for {
        m, err := r.FetchMessage(context.Background())
        if err != nil {
            return err // 返回给外层判断
        }
        // ... 处理 ...
    }
}
```

错误分类：
- **temporary error**（网络抖动、leader 切换、连接 reset）：指数退避重连
- **fatal error**（broker 地址错误、认证失败、权限不足）：直接退出进程，由容器编排重启

> **💡 代码细节（原始代码的问题）：**
>
> 见 `pkg/kafka/client.go:63`：
> ```go
> if err != nil {
>     log.Error("从 Kafka 读取消息失败", err)
>     break  // 退出后没有任何重启逻辑
> }
> ```
>
> `break` 退出 for 循环后，`r.Close()` 被调用，goroutine 结束。调用方 `StartConsumer` 没有返回值，上层也没有重启逻辑。
>
> 生产环境下这意味着：网络抖动 → 消费者永久停止 → 文件处理积压 → 用户等待但无响应 → 只能重启整个服务实例才能恢复。

---

### Q8（综合）：这套方案整体怎么描述

**面试回答：**

整套 Kafka 消费可靠性方案可以分三层：

1. **投递层（Kafka）**：手动 offset commit，处理成功才提交，保证 at-least-once 投递语义；消费者 supervisor 保证单点故障可自愈
2. **幂等层（MySQL 状态机）**：`SELECT FOR UPDATE` + `pending → processing → completed` 状态机，保证同一文件不被多个消费者并发处理；turnID/partition+offset 唯一索引保证重投后不重复落库
3. **恢复层（超时回滚，待完善）**：`processing` 状态 heartbeat 超时回滚，防止崩溃导致的卡死

当前实现覆盖了第 1 层和第 2 层的核心路径；第 3 层（heartbeat 超时回滚）和第 1 层的 supervisor 重连逻辑是已知缺口，需要补。

---

## 正式面试框架

```
Kafka 消费可靠性
│
├── 1. 投递语义基础
│   ├── Q1: 为什么手动 commit？（at-least-once vs at-most-once）
│   └── Q2: FetchMessage 不 commit 时行为？（追问：重试怎么触发）
│
├── 2. 重试机制的正确性
│   ├── Q3: retry key 用 FileMD5 的碰撞 bug
│   │   └── → 正确答案：partition + offset 唯一标识
│   └── Q6: Redis 计数 vs MySQL 状态机的逻辑冲突
│       └── → 现状：已删 Redis，只用状态机，单一职责
│
├── 3. 幂等性保证（核心）
│   ├── Q4: SELECT FOR UPDATE + 状态机完整流程
│   │   ├── 谁在哪一层？（不在 kafka/client.go，在处理服务层）
│   │   ├── 短事务范围（只锁"检查+改状态"，不锁整个处理）
│   │   └── 防的是什么并发？（rebalance 后两个消费者争同一条消息）
│   ├── Q5a: processing 卡死 → lease/heartbeat 修法
│   ├── Q5b: 事务范围的两种错误极端
│   └── Q5c: rebalance 时序的具体行为
│
└── 4. 消费者自身的可靠性
    └── Q7: break on error → supervisor 双层循环 + 错误分类退避
        ├── temporary error → 指数退避重连
        └── fatal error → fail-fast，交给容器编排
```

**面试深挖逻辑：**

- 面试官通常先问**机制**（手动 commit 有什么用），再问**边界**（FetchMessage 实际行为），再找**代码里的 bug**（retry key），最后问**你没做的**（processing 卡死、consumer 死亡）
- "你做了什么"容易准备，"你没做的但应该做的"才是区分度所在
- 三个主动承认的缺口要能说出修法：
  1. `processing` 卡死 → heartbeat 超时回滚
  2. consumer `break` → supervisor + 退避
  3. retry key 碰撞 → partition + offset
