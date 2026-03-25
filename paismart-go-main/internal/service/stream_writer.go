package service

// StreamWriter 抽象流式输出能力。
// AIHelper / ChatService 只依赖这个接口，不直接依赖 websocket.Conn。
type StreamWriter interface {
	WriteChunk(content string) error
	WriteDone() error
	WriteError(message string) error
}
