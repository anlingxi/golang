package handler

import (
	"encoding/json"
	"time"

	"pai-smart-go/internal/service"

	"github.com/gorilla/websocket"
)

var _ service.StreamWriter = (*WebSocketStreamWriter)(nil)

type WebSocketStreamWriter struct {
	conn *websocket.Conn
}

func NewWebSocketStreamWriter(conn *websocket.Conn) *WebSocketStreamWriter {
	return &WebSocketStreamWriter{
		conn: conn,
	}
}

// WriteChunk 实现了 StreamWriter 接口，负责将增量内容通过 WebSocket 发送给前端。
// 每次调用都会构造一个包含内容和时间戳的 JSON 消息，并通过 WebSocket 发送。
// 和sserver不同，WebSocket 是双向通信的，所以这里直接发送文本消息，不需要像 SSE 那样构造特定格式。

func (w *WebSocketStreamWriter) WriteChunk(content string) error {
	resp := map[string]interface{}{
		"type":      "text",
		"content":   content,
		"timestamp": time.Now().UnixMilli(),
		"date":      time.Now().Format("2006-01-02T15:04:05"),
	}
	// marshal 成 JSON 格式发送，前端收到后根据 type 字段区分消息类型，进行不同处理
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return w.conn.WriteMessage(websocket.TextMessage, b)
}

func (w *WebSocketStreamWriter) WriteDone() error {
	resp := map[string]interface{}{
		"type":      "completion",
		"status":    "finished",
		"message":   "响应已完成",
		"timestamp": time.Now().UnixMilli(),
		"date":      time.Now().Format("2006-01-02T15:04:05"),
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return w.conn.WriteMessage(websocket.TextMessage, b)
}

func (w *WebSocketStreamWriter) WriteError(message string) error {
	resp := map[string]interface{}{
		"type":      "error",
		"error":     message,
		"timestamp": time.Now().UnixMilli(),
		"date":      time.Now().Format("2006-01-02T15:04:05"),
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return w.conn.WriteMessage(websocket.TextMessage, b)
}
