package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"pai-smart-go/internal/eino/factory"
	einotools "pai-smart-go/internal/eino/tools"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/service"
)

type AgentHandler struct {
	products    factory.AIProducts
	toolBuilder einotools.Builder
}

func NewAgentHandler(products factory.AIProducts, toolBuilder einotools.Builder) *AgentHandler {
	return &AgentHandler{products: products, toolBuilder: toolBuilder}
}

// AgentChat POST /api/v1/agent/chat
func (h *AgentHandler) AgentChat(c *gin.Context) {
	user := c.MustGet("user").(*model.User)

	var req struct {
		Message string `json:"message" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tools, err := h.toolBuilder.Build(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "工具集初始化失败"})
		return
	}

	agent, err := factory.NewKnowledgeAgent(c.Request.Context(), h.products, tools)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Agent初始化失败"})
		return
	}

	agentSvc := service.NewAgentChatService(agent)
	answer, err := agentSvc.Chat(c.Request.Context(), req.Message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"answer": answer})
}
