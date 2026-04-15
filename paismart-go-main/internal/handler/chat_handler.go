package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"pai-smart-go/internal/ai/helper"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/internal/service"
	"pai-smart-go/pkg/log"
	"pai-smart-go/pkg/token"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

type CancelReason string

const (
	cancelReasonNone       CancelReason = ""
	cancelReasonStopped    CancelReason = "stopped"
	cancelReasonSuperseded CancelReason = "superseded"
	finishReasonCompleted               = "completed"
	finishReasonStopped                 = "stopped"
	finishReasonSuperseded              = "superseded"
)

type wsClientMessage struct {
	Type           string `json:"type"`
	RequestID      string `json:"requestId"`
	ConversationID string `json:"conversationId"`
	Message        string `json:"message"`
}

// activeRequest 代表一个正在处理的请求，包含请求ID、会话ID、取消函数和完成信号等信息。
type activeRequest struct {
	requestID      string
	conversationID string
	cancel         context.CancelFunc
	done           chan struct{}

	mu     sync.RWMutex
	reason CancelReason
}

func (r *activeRequest) setReason(reason CancelReason) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reason = reason
}

func (r *activeRequest) getReason() CancelReason {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.reason
}

// 一个会话对应一个 WebSocket 连接，连接内可以有多个请求（requestId），但同一时刻只能有一个活跃请求在处理。
type chatSession struct {
	conn             *websocket.Conn
	writeMu          sync.Mutex
	stateMu          sync.Mutex
	active           *activeRequest
	lastConversation string
}

func (s *chatSession) writeJSON(payload any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteJSON(payload)
}

func (s *chatSession) setActive(active *activeRequest) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.active = active
	s.lastConversation = active.conversationID
}

func (s *chatSession) clearActive(active *activeRequest) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.active == active {
		s.active = nil
		if active.conversationID != "" {
			s.lastConversation = active.conversationID
		}
	}
}

func (s *chatSession) currentActive() *activeRequest {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.active
}

func (s *chatSession) currentConversationID() string {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.active != nil && s.active.conversationID != "" {
		return s.active.conversationID
	}
	return s.lastConversation
}

// 如果当前有活跃请求且 requestID 为空或与活跃请求的 requestID 匹配，则取消该请求并等待其完成。
// 返回被取消的 activeRequest，以便调用者可以获取取消原因等信息。
func (s *chatSession) cancelActive(reason CancelReason, requestID string) *activeRequest {
	s.stateMu.Lock()
	active := s.active
	// 只有当 active 不为 nil 且 requestID 为空或与 active 的 requestID 匹配时，才执行取消操作。这确保了我们不会错误地取消一个不相关的请求。
	if active == nil || (requestID != "" && active.requestID != requestID) {
		s.stateMu.Unlock()
		return nil
	}
	active.setReason(reason)
	cancel := active.cancel
	done := active.done
	s.stateMu.Unlock()

	cancel()
	<-done
	return active
}

// ChatHandler 负责处理 WebSocket 聊天连接。
type ChatHandler struct {
	helperManager    *helper.Manager
	conversationRepo repository.ConversationRepository
	userService      service.UserService
	jwtManager       *token.JWTManager
}

// NewChatHandler 创建一个新的 ChatHandler。
func NewChatHandler(
	helperManager *helper.Manager,
	conversationRepo repository.ConversationRepository,
	userService service.UserService,
	jwtManager *token.JWTManager,
) *ChatHandler {
	return &ChatHandler{
		helperManager:    helperManager,
		conversationRepo: conversationRepo,
		userService:      userService,
		jwtManager:       jwtManager,
	}
}

// Handle 处理一个传入的 WebSocket 连接。
func (h *ChatHandler) Handle(c *gin.Context) {
	// jwt 验证
	tokenString := c.Param("token")
	claims, err := h.jwtManager.VerifyToken(tokenString)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"message": "无效的 token",
			"data":    nil,
		})
		return
	}

	user, err := h.userService.GetProfile(claims.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "无法获取用户信息",
			"data":    nil,
		})
		return
	}
	// 升级http到websocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error("WebSocket 升级失败", err)
		return
	}
	defer conn.Close()

	// 创建一个独立的上下文用于管理整个连接的生命周期，连接关闭时取消该上下文，以便所有相关的请求都能及时停止
	connCtx, cancelConn := context.WithCancel(context.Background())
	defer cancelConn()

	session := &chatSession{
		conn: conn,
	}

	log.Infof("WebSocket 连接已建立，用户: %s", claims.Username)

	// 连接建立后进入消息处理循环，直到连接关闭
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Warnf("从 WebSocket 读取消息失败: %v", err)
			break
		}

		var req wsClientMessage
		if err := json.Unmarshal(message, &req); err != nil {
			_ = session.writeJSON(gin.H{
				"type":    "chat.error",
				"code":    "invalid_payload",
				"message": "消息格式无效",
			})
			continue
		}
		// 根据消息类型处理不同的逻辑，目前支持 ping、chat.send 和 chat.stop 三种类型
		switch req.Type {
		case "ping":
			_ = session.writeJSON(gin.H{"type": "pong"})
		case "chat.stop":
			active := session.cancelActive(cancelReasonStopped, strings.TrimSpace(req.RequestID))
			if active == nil {
				_ = session.writeJSON(gin.H{
					"type":      "chat.error",
					"requestId": strings.TrimSpace(req.RequestID),
					"code":      "request_not_found",
					"message":   "没有可停止的请求",
				})
			}
		case "chat.send":
			req.Message = strings.TrimSpace(req.Message)
			req.RequestID = strings.TrimSpace(req.RequestID)
			req.ConversationID = strings.TrimSpace(req.ConversationID)

			if req.RequestID == "" {
				_ = session.writeJSON(gin.H{
					"type":    "chat.error",
					"code":    "missing_request_id",
					"message": "requestId 不能为空",
				})
				continue
			}
			if req.Message == "" {
				_ = session.writeJSON(gin.H{
					"type":      "chat.error",
					"requestId": req.RequestID,
					"code":      "empty_message",
					"message":   "消息不能为空",
				})
				continue
			}
			// 解析会话ID的优先级：请求中的 conversationId > 当前活跃请求的 conversationId > session 记录的最后一个 conversationId > 新建一个 conversationId
			fallbackConversation := session.currentConversationID()
			if session.currentActive() != nil {
				session.cancelActive(cancelReasonSuperseded, "")
			}

			conversationID, err := h.resolveConversationID(connCtx, user.ID, req.ConversationID, fallbackConversation)
			if err != nil {
				_ = session.writeJSON(gin.H{
					"type":      "chat.error",
					"requestId": req.RequestID,
					"code":      "conversation_init_failed",
					"message":   "会话初始化失败，请稍后重试",
				})
				continue
			}

			active := &activeRequest{
				requestID:      req.RequestID,
				conversationID: conversationID,
				done:           make(chan struct{}),
			}

			// 这个取消和循环外面的区别 在于，这个取消是针对具体的请求的，而循环外面的取消是针对整个连接的。当一个新的请求到来时，我们取消当前活跃的请求（如果有的话），并启动一个新的请求处理流程。
			reqCtx, reqCancel := context.WithCancel(connCtx)
			active.cancel = reqCancel
			session.setActive(active)

			go h.runGeneration(reqCtx, session, active, user, req.Message)
		default:
			_ = session.writeJSON(gin.H{
				"type":      "chat.error",
				"requestId": strings.TrimSpace(req.RequestID),
				"code":      "unsupported_message_type",
				"message":   "不支持的消息类型",
			})
		}
	}

	cancelConn()
	session.cancelActive(cancelReasonStopped, "")
}

func (h *ChatHandler) resolveConversationID(
	ctx context.Context,
	userID uint,
	requestConversationID string,
	fallbackConversationID string,
) (string, error) {
	if requestConversationID != "" {
		return requestConversationID, nil
	}
	if fallbackConversationID != "" {
		return fallbackConversationID, nil
	}
	return h.conversationRepo.GetOrCreateConversationID(ctx, userID)
}

func (h *ChatHandler) runGeneration(
	ctx context.Context,
	session *chatSession,
	active *activeRequest,
	user *model.User,
	message string,
) {
	defer func() {
		session.clearActive(active)
		close(active.done)
	}()

	if err := session.writeJSON(gin.H{
		"type":           "chat.accepted",
		"requestId":      active.requestID,
		"conversationId": active.conversationID,
	}); err != nil {
		log.Warnf("发送 accepted 消息失败: %v", err)
		active.setReason(cancelReasonStopped)
		active.cancel()
		return
	}

	aiHelper, err := h.helperManager.GetOrCreate(user.ID, active.conversationID)
	if err != nil {
		_ = session.writeJSON(gin.H{
			"type":      "chat.error",
			"requestId": active.requestID,
			"code":      "runtime_init_failed",
			"message":   "会话运行时初始化失败，请稍后重试",
		})
		return
	}

	writer := NewWebSocketStreamWriter(session, active.requestID)
	err = aiHelper.StreamResponse(ctx, user, message, writer)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			_ = session.writeJSON(gin.H{
				"type":         "chat.completed",
				"requestId":    active.requestID,
				"finishReason": finishReasonFromCancelReason(active.getReason()),
			})
			return
		}

		log.Errorf("处理流式响应失败: %v", err)
		_ = session.writeJSON(gin.H{
			"type":      "chat.error",
			"requestId": active.requestID,
			"code":      "ai_unavailable",
			"message":   "AI服务暂时不可用，请稍后重试",
		})
		return
	}

	_ = session.writeJSON(gin.H{
		"type":         "chat.completed",
		"requestId":    active.requestID,
		"finishReason": finishReasonCompleted,
	})
}

func finishReasonFromCancelReason(reason CancelReason) string {
	switch reason {
	case cancelReasonSuperseded:
		return finishReasonSuperseded
	case cancelReasonStopped:
		return finishReasonStopped
	default:
		return finishReasonStopped
	}
}
