// Package main 是应用程序的入口点。
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"pai-smart-go/internal/ai/helper"
	aihistory "pai-smart-go/internal/ai/history"
	"pai-smart-go/internal/config"
	einocallbacks "pai-smart-go/internal/eino/callbacks"
	documentbuilder "pai-smart-go/internal/eino/document/builder"
	"pai-smart-go/internal/eino/factory"
	einotools "pai-smart-go/internal/eino/tools"
	"pai-smart-go/internal/handler"
	"pai-smart-go/internal/infra/cache"
	"pai-smart-go/internal/infra/mq/rabbitmq"
	"pai-smart-go/internal/middleware"
	"pai-smart-go/internal/pipeline"
	"pai-smart-go/internal/repository"
	"pai-smart-go/internal/router"
	"pai-smart-go/internal/seed"
	"pai-smart-go/internal/service"
	"pai-smart-go/pkg/database"
	"pai-smart-go/pkg/embedding"
	"pai-smart-go/pkg/es"
	"pai-smart-go/pkg/kafka"
	"pai-smart-go/pkg/log"
	"pai-smart-go/pkg/storage"
	"pai-smart-go/pkg/tika"
	"pai-smart-go/pkg/token"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// 1. 初始化配置
	config.Init("./configs/config.yaml")
	cfg := config.Conf

	// 2. 初始化日志记录器
	log.Init(cfg.Log.Level, cfg.Log.Format, cfg.Log.OutputPath)
	defer log.Sync() // 确保在程序退出时刷新所有缓冲的日志条目
	log.Info("日志记录器初始化成功")

	// 3. 初始化数据库和 Redis
	database.InitMySQL(cfg.Database.MySQL.DSN)
	database.InitRedis(cfg.Database.Redis.Addr, cfg.Database.Redis.Password, cfg.Database.Redis.DB)
	storage.InitMinIO(cfg.MinIO)
	err := es.InitES(cfg.Elasticsearch)
	if err != nil {
		log.Errorf("es 初始化失败 %s", err)
		return
	}
	kafka.InitProducer(cfg.Kafka)

	// 4. 初始化 Repository
	userRepository := repository.NewUserRepository(database.DB)
	orgTagRepo := repository.NewOrgTagRepository(database.DB)
	uploadRepo := repository.NewUploadRepository(database.DB, database.RDB)
	conversationRepo := repository.NewConversationRepository(database.RDB)
	docVectorRepo := repository.NewDocumentVectorRepository(database.DB)

	// 5. 初始化 Service (依赖注入)
	jwtManager := token.NewJWTManager(cfg.JWT.Secret, cfg.JWT.AccessTokenExpireHours, cfg.JWT.RefreshTokenExpireDays)
	tikaClient := tika.NewClient(cfg.Tika)
	embeddingClient := embedding.NewClient(cfg.Embedding)

	userService := service.NewUserService(userRepository, orgTagRepo, jwtManager)
	adminService := service.NewAdminService(orgTagRepo, userRepository, conversationRepo)
	uploadService := service.NewUploadService(uploadRepo, userRepository, cfg.MinIO)
	documentService := service.NewDocumentService(uploadRepo, userRepository, orgTagRepo, cfg.MinIO, tikaClient)
	searchService := service.NewSearchService(embeddingClient, es.ESClient, userService, uploadRepo, cfg.Elasticsearch.IndexName)
	conversationService := service.NewConversationService(conversationRepo)

	// 新聊天主链：ChatModel -> ChatService -> HistoryStore -> HelperFactory -> HelperManager
	registry := factory.NewRegistry(cfg.Eino)
	aiFactory, err := registry.GetFactory(context.Background())
	if err != nil {
		log.Errorf("get ai factory failed: %v", err)
		return
	}

	products, err := aiFactory.Create(context.Background())
	if err != nil {
		log.Errorf("create ai products failed: %v", err)
		return
	}
	// 从产品族中获取 ChatModel 实例，传入 ChatService 以供 Helper 使用
	// 从产品族中获取 ChatModel 实例，传入 ChatService 以供 Helper 使用
	chatModel := products.ChatModel()
	callbackManager := einocallbacks.NewManager(cfg.Eino.Callback)

	chatService := service.NewChatService(
		searchService,
		chatModel,
		callbackManager,
		cfg.Eino,
	)

	// 先用 noop loader/persister 打通新 history 注入链
	recentTTL := 30 * time.Minute

	historyCache := cache.NewRedisHistoryCache(database.RDB)

	sessionRepo := repository.NewSessionRepository(database.DB)
	msgRepo := repository.NewConversationMsgRepository(database.DB)
	turnRepo := repository.NewConversationTurnRepository(database.DB)

	loader := aihistory.NewCacheThenDBLoader(historyCache, msgRepo, recentTTL)

	// 真实 DB 落库逻辑，给 consumer 复用
	syncPersister := aihistory.NewSyncDBPersister(historyCache, sessionRepo, msgRepo, turnRepo, recentTTL)

	// RabbitMQ config -> infra config
	rabbitCfg := rabbitmq.Config{
		URL:             cfg.RabbitMQ.URL,
		Exchange:        cfg.RabbitMQ.Exchange,
		ExchangeType:    cfg.RabbitMQ.ExchangeType,
		RoutingKey:      cfg.RabbitMQ.RoutingKey,
		Queue:           cfg.RabbitMQ.Queue,
		RetryQueue:      cfg.RabbitMQ.RetryQueue,
		DeadLetterQueue: cfg.RabbitMQ.DeadLetterQueue,
		PrefetchCount:   cfg.RabbitMQ.PrefetchCount,
		ConsumerTag:     cfg.RabbitMQ.ConsumerTag,
	}

	rabbitProducer, err := rabbitmq.NewProducer(rabbitCfg)
	if err != nil {
		log.Errorf("init rabbitmq producer failed: %v", err)
		return
	}
	defer rabbitProducer.Close()

	taskPublisher := rabbitProducer

	asyncPersister := aihistory.NewCacheAndAsyncDBPersister(
		historyCache,
		taskPublisher,
		recentTTL,
	)
	// 生产者异步生产消息
	historyManager := aihistory.NewManager(loader, asyncPersister)

	helperFactory := helper.NewDefaultFactory(chatService, historyManager)
	helperManager := helper.NewManager(helperFactory)

	// 后台 consumer
	taskHandler := aihistory.NewPersistTurnTaskHandler(turnRepo, syncPersister)
	rabbitConsumer, err := rabbitmq.NewConsumer(rabbitCfg, taskHandler)
	if err != nil {
		log.Errorf("init rabbitmq consumer failed: %v", err)
		return
	}
	defer rabbitConsumer.Close()
	// 这里直接开一个 goroutine 来启动 RabbitMQ 消费者，实际项目中可以考虑更优雅的方式来管理消费者的生命周期
	// 实际项目中可以换成更优雅的方式来管理消费者的生命周期，比如使用 errgroup 或者在 main 函数中监听退出信号时优雅关闭消费者等。
	go func() {
		if err := rabbitConsumer.Start(context.Background()); err != nil && err != context.Canceled {
			log.Errorf("rabbitmq consumer stopped with error: %v", err)
		}
	}()

	// 6. 初始化文件处理管道 (Processor)
	docPipeline, err := documentbuilder.NewPipeline(
		context.Background(),
		cfg,           // config.Config
		es.ESClient,   // *elasticsearch.Client
		products,      // factory.AIProducts（第 85 行已创建）
		docVectorRepo, // repository.DocumentVectorRepository
	)
	if err != nil {
		log.Errorf("failed to init document pipeline: %v", err)
		return
	}

	processor := pipeline.NewProcessor(docPipeline)

	// 7. 启动后台 Kafka 消费者
	go kafka.StartConsumer(cfg.Kafka, processor)

	// 7.1 初始化导入 initfile 目录：模拟真实上传 + 合并（全员可见，归属 admin），已导入则跳过
	initCtx, cancelInit := context.WithCancel(context.Background())
	defer cancelInit()
	go seed.InitSeedFiles(initCtx, "initfile", userRepository, uploadService)

	// 8. 设置 Gin 模式并创建路由引擎
	gin.SetMode(cfg.Server.Mode)
	r := gin.New() // 使用 New() 创建一个不带默认中间件的引擎
	// 添加我们自定义的日志中间件和 Gin 的 Recovery 中间件
	r.Use(middleware.RequestLogger(), gin.Recovery())

	// 9. 注册路由
	toolBuilder, err := einotools.NewBuilder(cfg.Eino.Agent.Tools, searchService, documentService)
	if err != nil {
		log.Errorf("init agent tool builder failed: %v", err)
		return
	}

	agentHandler := handler.NewAgentHandler(products, toolBuilder)

	router.RegisterRoutes(
		r,
		jwtManager,
		userService,
		adminService,
		uploadService,
		documentService,
		searchService,
		conversationService,
		conversationRepo,
		helperManager,
		agentHandler,
	)

	// 启动 HTTP 服务器并实现优雅停机
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.Server.Port),
		Handler: r,
	}

	go func() {
		log.Infof("服务启动于 %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务监听失败: %s\n", err)
		}
	}()

	// 等待中断信号以实现优雅停机
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("接收到停机信号，正在关闭服务...")

	// 设置一个5秒的超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 关闭 HTTP 服务器
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("HTTP 服务器关闭失败: %v", err)
	}

	// 在优雅停机逻辑中，我们不需要手动关闭 Kafka 消费者，
	// 因为 StartConsumer 是一个循环，会在程序退出时自然结束。
	// 如果需要更精细的控制，可以在 StartConsumer 中实现一个关闭通道。
	log.Info("服务已优雅关闭")
}
