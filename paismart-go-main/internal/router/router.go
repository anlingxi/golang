package router

import (
	"pai-smart-go/internal/ai/helper"
	"pai-smart-go/internal/handler"
	"pai-smart-go/internal/middleware"
	"pai-smart-go/internal/repository"
	"pai-smart-go/internal/service"
	"pai-smart-go/pkg/token"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(
	r *gin.Engine,
	jwtManager *token.JWTManager,
	userService service.UserService,
	adminService service.AdminService,
	uploadService service.UploadService,
	documentService service.DocumentService,
	searchService service.SearchService,
	conversationService service.ConversationService,
	conversationRepo repository.ConversationRepository,
	helperManager *helper.Manager,
	agentHandler *handler.AgentHandler,
) {
	// 先统一创建一次 ChatHandler，避免重复 new
	chatHandler := handler.NewChatHandler(
		helperManager,
		conversationRepo,
		userService,
		jwtManager,
	)

	apiV1 := r.Group("/api/v1")
	{
		// Auth 路由组
		auth := apiV1.Group("/auth")
		{
			auth.POST("/refreshToken", handler.NewAuthHandler(userService).RefreshToken)
		}

		users := apiV1.Group("/users")
		{
			// 无需认证的路由
			users.POST("/register", handler.NewUserHandler(userService).Register)
			users.POST("/login", handler.NewUserHandler(userService).Login)

			// 需要认证的路由
			authed := users.Group("/")
			authed.Use(middleware.AuthMiddleware(jwtManager, userService))
			{
				authed.GET("/me", handler.NewUserHandler(userService).GetProfile)
				authed.POST("/logout", handler.NewUserHandler(userService).Logout)
				authed.PUT("/primary-org", handler.NewUserHandler(userService).SetPrimaryOrg)
				authed.GET("/org-tags", handler.NewUserHandler(userService).GetUserOrgTags)
			}
		}

		// Upload 路由组
		upload := apiV1.Group("/upload")
		upload.Use(middleware.AuthMiddleware(jwtManager, userService))
		{
			upload.POST("/check", handler.NewUploadHandler(uploadService).CheckFile)
			upload.POST("/chunk", handler.NewUploadHandler(uploadService).UploadChunk)
			upload.POST("/merge", handler.NewUploadHandler(uploadService).MergeChunks)
			upload.GET("/status", handler.NewUploadHandler(uploadService).GetUploadStatus)
			upload.GET("/supported-types", handler.NewUploadHandler(uploadService).GetSupportedFileTypes)
			upload.POST("/fast-upload", handler.NewUploadHandler(uploadService).FastUpload)
		}

		// Document 路由组
		documents := apiV1.Group("/documents")
		documents.Use(middleware.AuthMiddleware(jwtManager, userService))
		{
			documents.GET("/accessible", handler.NewDocumentHandler(documentService, userService).ListAccessibleFiles)
			documents.GET("/uploads", handler.NewDocumentHandler(documentService, userService).ListUploadedFiles)
			documents.DELETE("/:fileMd5", handler.NewDocumentHandler(documentService, userService).DeleteDocument)
			documents.GET("/download", handler.NewDocumentHandler(documentService, userService).GenerateDownloadURL)
			documents.GET("/preview", handler.NewDocumentHandler(documentService, userService).PreviewFile)
		}

		// Search 路由组
		search := apiV1.Group("/search")
		search.Use(middleware.AuthMiddleware(jwtManager, userService))
		{
			search.GET("/hybrid", handler.NewSearchHandler(searchService).HybridSearch)
		}

		// Conversation 路由组
		conversation := apiV1.Group("/users/conversation")
		conversation.Use(middleware.AuthMiddleware(jwtManager, userService))
		{
			conversation.GET("", handler.NewConversationHandler(conversationService).GetConversations)
		}

		agent := apiV1.Group("/agent")
		agent.Use(middleware.AuthMiddleware(jwtManager, userService))
		{
			agent.POST("/chat", agentHandler.AgentChat)
		}

		// WebSocket 主连接
		r.GET("/chat/:token", chatHandler.Handle)

		admin := apiV1.Group("/admin")
		admin.Use(middleware.AuthMiddleware(jwtManager, userService), middleware.AdminAuthMiddleware())
		{
			admin.GET("/users/list", handler.NewAdminHandler(adminService, userService).ListUsers)
			admin.PUT("/users/:userId/org-tags", handler.NewAdminHandler(adminService, userService).AssignOrgTagsToUser)
			admin.GET("/conversation", handler.NewAdminHandler(adminService, userService).GetAllConversations)

			orgTags := admin.Group("/org-tags")
			{
				orgTags.POST("", handler.NewAdminHandler(adminService, userService).CreateOrganizationTag)
				orgTags.GET("", handler.NewAdminHandler(adminService, userService).ListOrganizationTags)
				orgTags.GET("/tree", handler.NewAdminHandler(adminService, userService).GetOrganizationTagTree)
				orgTags.PUT("/:id", handler.NewAdminHandler(adminService, userService).UpdateOrganizationTag)
				orgTags.DELETE("/:id", handler.NewAdminHandler(adminService, userService).DeleteOrganizationTag)
			}
		}
	}
}
