package callbacks

// 自定义RunMeta结构体，包含场景、阶段、提供商、模型、用户ID、会话ID和请求ID等信息
// 为扩展性和灵活性设计，支持不同的场景和阶段组合，以及不同的提供商和模型
// 例如，场景可以是 "chat"，阶段可以是 "generate" 或 "retrieve"，提供商可以是 "openai" 或 "azure"，模型可以是 "gpt-3.5-turbo" 等
type RunMeta struct {
	Scene          string
	Stage          string
	Provider       string
	Model          string
	UserID         uint
	ConversationID string
	RequestID      string
}

// 组件名称，格式为 scene.stage，如果 stage 为空，则仅为 scene；如果 scene 和 stage 都为空，则为 "unknown"
func (m RunMeta) ComponentName() string {
	if m.Scene == "" && m.Stage == "" {
		return "unknown"
	}
	if m.Stage == "" {
		return m.Scene
	}
	return m.Scene + "." + m.Stage
}

// 设置阶段
func (m RunMeta) WithStage(stage string) RunMeta {
	m.Stage = stage
	return m
}
