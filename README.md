Go 版派聪明（PaiSmart-Go）是一个企业级的 AI 知识库管理系统，采用 RAG 技术提供智能文档处理和检索能力。核心技术栈包括：

- Go 1.23+、模块化目录：`cmd/` `internal/` `pkg/`；分层：`handler/service/repository`
- 配置/日志/关停：Viper、Zap（结构化日志）、Gin + Context 优雅停机
- Gin（路由分组/中间件）、Gorilla WebSocket（双向通信、增量写出、停止指令）
- JWT（access/refresh）、基于 `org_tag` 的层级聚合，检索期过滤（should + minimum_should_match）
- MySQL 8 + GORM（文件/分片/向量等元数据持久化）、Redis 7（分片进度与重试计数）
- MinIO：分片对象存储；单分片 Copy、多分片 Compose；合并后后台清理分片对象
- Kafka（segmentio/kafka-go）：生产/消费、失败阈值重试、手动提交 offset
- 任务解耦：`TaskProcessor` 接口承载解析/向量化/索引流水线
- Apache Tika（HTTP 服务）：PDF/DOCX/PPT/XLS 等文本抽取
- 分块策略：固定窗口 + 重叠切分（提升语义覆盖）
- Elasticsearch 8：KNN 语义召回 + BM25 rescore + 短语兜底 should；索引含 `userId/orgTag/isPublic`
- Embedding：OpenAI 兼容协议，已适配 DashScope（维度 2048 可配）
- LLM：DeepSeek Chat 流式；可按同协议切换本地 Ollama
- Docker 容器化：一键拉起 MySQL/Redis/ES/Kafka/MinIO/Tika
- 集中管理 LLM/Embedding/ES 等参数
