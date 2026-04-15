package handler

import (
	"time"

	"pai-smart-go/internal/service"
)

var _ service.StreamWriter = (*WebSocketStreamWriter)(nil)

type WebSocketStreamWriter struct {
	session   *chatSession
	requestID string
}

func NewWebSocketStreamWriter(session *chatSession, requestID string) *WebSocketStreamWriter {
	return &WebSocketStreamWriter{
		session:   session,
		requestID: requestID,
	}
}

func (w *WebSocketStreamWriter) WriteChunk(content string) error {
	return w.session.writeJSON(map[string]interface{}{
		"type":      "chat.delta",
		"requestId": w.requestID,
		"delta":     content,
		"timestamp": time.Now().UnixMilli(),
	})
}

func (w *WebSocketStreamWriter) WriteDone() error {
	return w.session.writeJSON(map[string]interface{}{
		"type":         "chat.completed",
		"requestId":    w.requestID,
		"finishReason": finishReasonCompleted,
		"timestamp":    time.Now().UnixMilli(),
	})
}

func (w *WebSocketStreamWriter) WriteError(message string) error {
	return w.session.writeJSON(map[string]interface{}{
		"type":      "chat.error",
		"requestId": w.requestID,
		"code":      "internal_error",
		"message":   message,
		"timestamp": time.Now().UnixMilli(),
	})
}

