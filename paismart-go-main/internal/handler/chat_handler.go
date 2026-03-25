package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"pai-smart-go/internal/ai/helper"
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

// ChatHandler 负责处理 WebSocket 聊天连接。
type ChatHandler struct {
	helperManager    *helper.Manager
	conversationRepo repository.ConversationRepository
	userService      service.UserService
	jwtManager       *token.JWTManager

	stopToken     string
	stopTokenLock sync.Mutex

	// 每连接停止标志
	stopFlags sync.Map // key: session pointer string, value: bool
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

// GetWebsocketStopToken 返回一个可用于停止流的令牌。
func (h *ChatHandler) GetWebsocketStopToken(c *gin.Context) {
	h.stopTokenLock.Lock()
	defer h.stopTokenLock.Unlock()

	h.stopToken = "WSS_STOP_CMD_" + token.GenerateRandomString(16)
	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "success",
		"data": gin.H{
			"cmdToken": h.stopToken,
		},
	})
}

// Handle 处理一个传入的 WebSocket 连接。
func (h *ChatHandler) Handle(c *gin.Context) {
	tokenString := c.Param("token")
	// 验证 token，获取用户信息
	claims, err := h.jwtManager.VerifyToken(tokenString)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"message": "无效的 token",
			"data":    nil,
		})
		return
	}

	// 根据 claims 获取用户信息
	user, err := h.userService.GetProfile(claims.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "无法获取用户信息",
			"data":    nil,
		})
		return
	}

	// 升级 HTTP 连接到 WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error("WebSocket 升级失败", err)
		return
	}
	defer conn.Close()

	log.Infof("WebSocket 连接已建立，用户: %s", claims.Username)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Warnf("从 WebSocket 读取消息失败: %v", err)
			break
		}
		log.Infof("收到 WebSocket 消息: %s", string(message))

		// 1) JSON 停止指令
		// 前端发送的停止指令格式为 JSON，包含 type 字段和 _internal_cmd_token 字段
		var ctrl map[string]interface{}
		if len(message) > 0 && message[0] == '{' {
			if err := json.Unmarshal(message, &ctrl); err == nil {
				if t, ok := ctrl["type"].(string); ok && t == "stop" {
					if tok, ok := ctrl["_internal_cmd_token"].(string); ok {
						h.stopTokenLock.Lock()
						valid := (tok == h.stopToken)
						h.stopTokenLock.Unlock()
						if valid {
							key := sessionKey(conn)
							// 设置当前连接的停止标志为 true，表示应该停止流式响应。
							h.stopFlags.Store(key, true)

							resp := map[string]interface{}{
								"type":      "stop",
								"message":   "响应已停止",
								"timestamp": time.Now().UnixMilli(),
								"date":      time.Now().Format("2006-01-02T15:04:05"),
							}
							b, _ := json.Marshal(resp)
							_ = conn.WriteMessage(websocket.TextMessage, b)
							continue
						}
					}
				}
			}
		}

		// 2) 旧停止令牌兼容
		h.stopTokenLock.Lock()
		stopTokenValue := h.stopToken
		h.stopTokenLock.Unlock()

		if string(message) == stopTokenValue {
			log.Info("收到停止指令，正在中断流式响应...")
			key := sessionKey(conn)
			h.stopFlags.Store(key, true)
			continue
		}

		// 3) 获取或创建会话 ID
		conversationID, err := h.conversationRepo.GetOrCreateConversationID(c.Request.Context(), user.ID)
		if err != nil {
			log.Errorf("获取或创建会话 ID 失败: %v", err)
			writer := NewWebSocketStreamWriter(conn)
			_ = writer.WriteError("会话初始化失败，请稍后重试")
			_ = writer.WriteDone()
			continue
		}

		// 4) 获取或创建 AIHelper
		aiHelper, err := h.helperManager.GetOrCreate(user.ID, conversationID)
		if err != nil {
			log.Errorf("获取或创建 AIHelper 失败: %v", err)
			writer := NewWebSocketStreamWriter(conn)
			_ = writer.WriteError("会话运行时初始化失败，请稍后重试")
			_ = writer.WriteDone()
			continue
		}

		// 5) 构造流式 writer
		writer := NewWebSocketStreamWriter(conn)
		// 构造 shouldStop 函数，检查当前连接的停止标志
		// 防止并发问题，应该在访问 stopFlags 时使用 sync.Map 的原子操作，避免锁竞争和死锁风险。
		// 回调函数在aihelper调用的时候，是如何知道conn和h的，命名已经不是这个函数中的了？回调函数是一个闭包，它捕获了外部函数中的变量，
		// 包括 conn 和 h。当我们在 aiHelper.StreamResponse 中传入 shouldStop 函数时，这个函数已经包含了对 conn 和 h 的引用，
		// 因此在 shouldStop 内部我们可以直接访问 conn 和 h 来检查停止标志。
		shouldStop := func() bool {
			key := sessionKey(conn)
			v, ok := h.stopFlags.Load(key)
			// 返回当前连接的停止标志，如果存在且为 true，就返回 true，表示应该停止流式响应。
			// 这里的return后代码，当前代码还会完成本次流式生成吗？会的，因为 shouldStop 只是一个检查函数，
			// 真正的流式生成逻辑是在 AIHelper.StreamResponse 中实现的，只要 shouldStop 返回 false，生成逻辑就会继续执行。
			// 一旦 shouldStop 返回 true，AIHelper.StreamResponse 就会停止生成新的内容，并结束流式响应。
			return ok && v.(bool)
		}

		// 清除旧停止标志
		// 怎么清理的？在 shouldStop 函数中，我们检查当前连接的停止标志，如果存在且为 true，就返回 true，表示应该停止流式响应。
		// 当我们收到新的消息时，我们会清除旧的停止标志，确保新的消息能够正常处理，而不会被旧的停止指令误伤。
		// 难道不应该在 shouldStop 函数中清除吗？不应该，因为 shouldStop 函数只是检查停止标志，而不是修改它。
		// 我们需要在收到新消息时清除旧的停止标志，确保新的消息能够正常处理。
		h.stopFlags.Delete(sessionKey(conn))

		// 6) 调用 AIHelper，正式进入新主链
		err = aiHelper.StreamResponse(
			c.Request.Context(),
			user,
			string(message),
			writer,
			shouldStop,
		)
		if err != nil {
			log.Errorf("处理流式响应失败: %v", err)
			_ = writer.WriteError("AI服务暂时不可用，请稍后重试")
			_ = writer.WriteDone()
			break
		}
	}
}

func sessionKey(conn *websocket.Conn) string {
	return fmt.Sprintf("%p", conn)
}
