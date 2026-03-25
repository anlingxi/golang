// Package config 负责加载和管理应用程序的配置。
package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// 全局配置变量，存储从配置文件加载的所有设置。
var Conf Config

// Config 是整个应用程序的配置结构体，与 config.yaml 文件结构对应。
type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	JWT           JWTConfig           `mapstructure:"jwt"`
	Log           LogConfig           `mapstructure:"log"`
	Kafka         KafkaConfig         `mapstructure:"kafka"`
	Tika          TikaConfig          `mapstructure:"tika"`
	Elasticsearch ElasticsearchConfig `mapstructure:"elasticsearch"`
	MinIO         MinIOConfig         `mapstructure:"minio"`
	Embedding     EmbeddingConfig     `mapstructure:"embedding"`
	LLM           LLMConfig           `mapstructure:"llm"`
	AI            AIConfig            `mapstructure:"ai"`
	Eino          EinoConfig          `mapstructure:"eino"`
	RabbitMQ      RabbitMQConfig      `mapstructure:"rabbitmq"`
}

// ServerConfig 存储服务器相关的配置。
type ServerConfig struct {
	Port string `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

// DatabaseConfig 存储所有数据库连接的配置。
type DatabaseConfig struct {
	MySQL MySQLConfig `mapstructure:"mysql"`
	Redis RedisConfig `mapstructure:"redis"`
}

// MySQLConfig 存储 MySQL 数据库的配置。
type MySQLConfig struct {
	DSN string `mapstructure:"dsn"`
}

// RedisConfig 存储 Redis 的配置。
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// JWTConfig 存储 JWT 相关的配置。
type JWTConfig struct {
	Secret                 string `mapstructure:"secret"`
	AccessTokenExpireHours int    `mapstructure:"access_token_expire_hours"`
	RefreshTokenExpireDays int    `mapstructure:"refresh_token_expire_days"`
}

// LogConfig 存储日志相关的配置。
type LogConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	OutputPath string `mapstructure:"output_path"`
}

// KafkaConfig 存储 Kafka 相关的配置。
type KafkaConfig struct {
	Brokers          string `mapstructure:"brokers"`
	Topic            string `mapstructure:"topic"`
	ChatHistoryTopic string `mapstructure:"chat_history_topic"`
}

// TikaConfig 存储 Tika 服务器相关的配置。
type TikaConfig struct {
	ServerURL string `mapstructure:"server_url"`
}

// ElasticsearchConfig 存储 Elasticsearch 相关的配置。
type ElasticsearchConfig struct {
	Addresses string `mapstructure:"addresses"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	IndexName string `mapstructure:"index_name"`
}

// MinIOConfig 存储 MinIO 对象存储的配置。
type MinIOConfig struct {
	Endpoint        string `mapstructure:"endpoint"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	UseSSL          bool   `mapstructure:"use_ssl"`
	BucketName      string `mapstructure:"bucket_name"`
}

// EmbeddingConfig 存储 Embedding 模型相关的配置。
type EmbeddingConfig struct {
	APIKey     string `mapstructure:"api_key"`
	BaseURL    string `mapstructure:"base_url"`
	Model      string `mapstructure:"model"`
	Dimensions int    `mapstructure:"dimensions"`
}

// LLMConfig 存储大语言模型相关的配置。
type LLMConfig struct {
	APIKey     string              `mapstructure:"api_key"`
	BaseURL    string              `mapstructure:"base_url"`
	Model      string              `mapstructure:"model"`
	Generation LLMGenerationConfig `mapstructure:"generation"`
	Prompt     LLMPromptConfig     `mapstructure:"prompt"`
}

// LLMGenerationConfig 配置生成相关参数（可选）。
type LLMGenerationConfig struct {
	Temperature float64 `mapstructure:"temperature"`
	TopP        float64 `mapstructure:"top_p"`
	MaxTokens   int     `mapstructure:"max_tokens"`
}

// LLMPromptConfig 配置系统提示与上下文包裹格式（可选）。
type LLMPromptConfig struct {
	Rules        string `mapstructure:"rules"`
	RefStart     string `mapstructure:"ref_start"`
	RefEnd       string `mapstructure:"ref_end"`
	NoResultText string `mapstructure:"no_result_text"`
}

// AIConfig 对齐 Java 的 ai.prompt/ai.generation（连字符键）
type AIConfig struct {
	Generation AIGenerationConfig `mapstructure:"generation"`
	Prompt     AIPromptConfig     `mapstructure:"prompt"`
}

type AIGenerationConfig struct {
	Temperature float64 `mapstructure:"temperature"`
	TopP        float64 `mapstructure:"top-p"`
	MaxTokens   int     `mapstructure:"max-tokens"`
}

type AIPromptConfig struct {
	Rules        string `mapstructure:"rules"`
	RefStart     string `mapstructure:"ref-start"`
	RefEnd       string `mapstructure:"ref-end"`
	NoResultText string `mapstructure:"no-result-text"`
}

// 新增eino相关配置
// 改为：
type EinoConfig struct {
	ChatModel EinoChatModelConfig `mapstructure:"chat_model"`
	Embedding EinoEmbeddingConfig `mapstructure:"embedding"` // ← 新增
	Callback  EinoCallbackConfig  `mapstructure:"callback"`
	Agent     EinoAgentConfig     `mapstructure:"agent"`
}

// 新增结构体：
type EinoEmbeddingConfig struct {
	Provider   string `mapstructure:"provider"` // "dashscope" / "openai"
	BaseURL    string `mapstructure:"base_url"` // "https://dashscope.aliyuncs.com/compatible-mode/v1"
	APIKey     string `mapstructure:"api_key"`
	Model      string `mapstructure:"model"`      // "text-embedding-v4"
	Dimensions *int   `mapstructure:"dimensions"` // 2048
}

type EinoChatModelConfig struct {
	Provider string `mapstructure:"provider"`
	Model    string `mapstructure:"model"`
	BaseURL  string `mapstructure:"base_url"`
	APIKey   string `mapstructure:"api_key"`
}

type EinoCallbackConfig struct {
	EnableLogging bool `mapstructure:"enable_logging"`
	EnableTrace   bool `mapstructure:"enable_trace"`
}

type EinoAgentConfig struct {
	Tools EinoAgentToolsConfig `mapstructure:"tools"`
}

type EinoAgentToolsConfig struct {
	KnowledgeSearch EinoKnowledgeSearchToolConfig `mapstructure:"knowledge_search"`
	ListDocuments   EinoListDocumentsToolConfig   `mapstructure:"list_documents"`
	CurrentTime     EinoCurrentTimeToolConfig     `mapstructure:"current_time"`
	WebSearch       EinoWebSearchToolConfig       `mapstructure:"web_search"`
	GitQuery        EinoGitQueryToolConfig        `mapstructure:"git_query"`
}

type EinoKnowledgeSearchToolConfig struct {
	Enabled     bool `mapstructure:"enabled"`
	DefaultTopK int  `mapstructure:"default_top_k"`
	MaxTopK     int  `mapstructure:"max_top_k"`
}

type EinoListDocumentsToolConfig struct {
	Enabled      bool `mapstructure:"enabled"`
	DefaultLimit int  `mapstructure:"default_limit"`
	MaxLimit     int  `mapstructure:"max_limit"`
}

type EinoCurrentTimeToolConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	DefaultTimezone string `mapstructure:"default_timezone"`
}

type EinoWebSearchToolConfig struct {
	Enabled           bool   `mapstructure:"enabled"`
	Provider          string `mapstructure:"provider"`
	BaseURL           string `mapstructure:"base_url"`
	APIKey            string `mapstructure:"api_key"`
	TimeoutSeconds    int    `mapstructure:"timeout_seconds"`
	ProxyURL          string `mapstructure:"proxy_url"`
	DefaultMaxResults int    `mapstructure:"default_max_results"`
	MaxMaxResults     int    `mapstructure:"max_max_results"`
	SearchDepth       string `mapstructure:"search_depth"`
}

type EinoGitQueryToolConfig struct {
	Enabled        bool     `mapstructure:"enabled"`
	BaseURL        string   `mapstructure:"base_url"`
	APIKey         string   `mapstructure:"api_key"`
	TimeoutSeconds int      `mapstructure:"timeout_seconds"`
	ProxyURL       string   `mapstructure:"proxy_url"`
	MaxFileChars   int      `mapstructure:"max_file_chars"`
	AllowRepos     []string `mapstructure:"allow_repos"`
}

type RabbitMQConfig struct {
	URL             string `mapstructure:"url"`
	Exchange        string `mapstructure:"exchange"`
	ExchangeType    string `mapstructure:"exchange_type"`
	RoutingKey      string `mapstructure:"routing_key"`
	Queue           string `mapstructure:"queue"`
	RetryQueue      string `mapstructure:"retry_queue"`
	DeadLetterQueue string `mapstructure:"dead_letter_queue"`
	PrefetchCount   int    `mapstructure:"prefetch_count"`
	ConsumerTag     string `mapstructure:"consumer_tag"`
}

// Init 初始化配置加载，从指定的路径读取 YAML 文件并解析到 Conf 变量中。
func Init(configPath string) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("读取配置文件失败: %w", err))
	}

	if err := viper.Unmarshal(&Conf); err != nil {
		panic(fmt.Errorf("无法将配置解析到结构体中: %w", err))
	}
}
// Package config 负责加载和管理应用程序的配置。
package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// 全局配置变量，存储从配置文件加载的所有设置。
var Conf Config

// Config 是整个应用程序的配置结构体，与 config.yaml 文件结构对应。
type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	JWT           JWTConfig           `mapstructure:"jwt"`
	Log           LogConfig           `mapstructure:"log"`
	Kafka         KafkaConfig         `mapstructure:"kafka"`
	Tika          TikaConfig          `mapstructure:"tika"`
	Elasticsearch ElasticsearchConfig `mapstructure:"elasticsearch"`
	MinIO         MinIOConfig         `mapstructure:"minio"`
	Embedding     EmbeddingConfig     `mapstructure:"embedding"`
	LLM           LLMConfig           `mapstructure:"llm"`
	AI            AIConfig            `mapstructure:"ai"`
	Eino          EinoConfig          `mapstructure:"eino"`
	RabbitMQ      RabbitMQConfig      `mapstructure:"rabbitmq"`
}

// ServerConfig 存储服务器相关的配置。
type ServerConfig struct {
	Port string `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

// DatabaseConfig 存储所有数据库连接的配置。
type DatabaseConfig struct {
	MySQL MySQLConfig `mapstructure:"mysql"`
	Redis RedisConfig `mapstructure:"redis"`
}

// MySQLConfig 存储 MySQL 数据库的配置。
type MySQLConfig struct {
	DSN string `mapstructure:"dsn"`
}

// RedisConfig 存储 Redis 的配置。
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// JWTConfig 存储 JWT 相关的配置。
type JWTConfig struct {
	Secret                 string `mapstructure:"secret"`
	AccessTokenExpireHours int    `mapstructure:"access_token_expire_hours"`
	RefreshTokenExpireDays int    `mapstructure:"refresh_token_expire_days"`
}

// LogConfig 存储日志相关的配置。
type LogConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	OutputPath string `mapstructure:"output_path"`
}

// KafkaConfig 存储 Kafka 相关的配置。
type KafkaConfig struct {
	Brokers          string `mapstructure:"brokers"`
	Topic            string `mapstructure:"topic"`
	ChatHistoryTopic string `mapstructure:"chat_history_topic"`
}

// TikaConfig 存储 Tika 服务器相关的配置。
type TikaConfig struct {
	ServerURL string `mapstructure:"server_url"`
}

// ElasticsearchConfig 存储 Elasticsearch 相关的配置。
type ElasticsearchConfig struct {
	Addresses string `mapstructure:"addresses"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	IndexName string `mapstructure:"index_name"`
}

// MinIOConfig 存储 MinIO 对象存储的配置。
type MinIOConfig struct {
	Endpoint        string `mapstructure:"endpoint"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	UseSSL          bool   `mapstructure:"use_ssl"`
	BucketName      string `mapstructure:"bucket_name"`
}

// EmbeddingConfig 存储 Embedding 模型相关的配置。
type EmbeddingConfig struct {
	APIKey     string `mapstructure:"api_key"`
	BaseURL    string `mapstructure:"base_url"`
	Model      string `mapstructure:"model"`
	Dimensions int    `mapstructure:"dimensions"`
}

// LLMConfig 存储大语言模型相关的配置。
type LLMConfig struct {
	APIKey     string              `mapstructure:"api_key"`
	BaseURL    string              `mapstructure:"base_url"`
	Model      string              `mapstructure:"model"`
	Generation LLMGenerationConfig `mapstructure:"generation"`
	Prompt     LLMPromptConfig     `mapstructure:"prompt"`
}

// LLMGenerationConfig 配置生成相关参数（可选）。
type LLMGenerationConfig struct {
	Temperature float64 `mapstructure:"temperature"`
	TopP        float64 `mapstructure:"top_p"`
	MaxTokens   int     `mapstructure:"max_tokens"`
}

// LLMPromptConfig 配置系统提示与上下文包裹格式（可选）。
type LLMPromptConfig struct {
	Rules        string `mapstructure:"rules"`
	RefStart     string `mapstructure:"ref_start"`
	RefEnd       string `mapstructure:"ref_end"`
	NoResultText string `mapstructure:"no_result_text"`
}

// AIConfig 对齐 Java 的 ai.prompt/ai.generation（连字符键）
type AIConfig struct {
	Generation AIGenerationConfig `mapstructure:"generation"`
	Prompt     AIPromptConfig     `mapstructure:"prompt"`
}

type AIGenerationConfig struct {
	Temperature float64 `mapstructure:"temperature"`
	TopP        float64 `mapstructure:"top-p"`
	MaxTokens   int     `mapstructure:"max-tokens"`
}

type AIPromptConfig struct {
	Rules        string `mapstructure:"rules"`
	RefStart     string `mapstructure:"ref-start"`
	RefEnd       string `mapstructure:"ref-end"`
	NoResultText string `mapstructure:"no-result-text"`
}

// 新增eino相关配置
type EinoConfig struct {
	ChatModel EinoChatModelConfig `mapstructure:"chat_model"`
	Callback  EinoCallbackConfig  `mapstructure:"callback"`
}

type EinoChatModelConfig struct {
	Provider string `mapstructure:"provider"`
	Model    string `mapstructure:"model"`
	BaseURL  string `mapstructure:"base_url"`
	APIKey   string `mapstructure:"api_key"`
}

type EinoCallbackConfig struct {
	EnableLogging bool `mapstructure:"enable_logging"`
	EnableTrace   bool `mapstructure:"enable_trace"`
}

type RabbitMQConfig struct {
	URL             string `mapstructure:"url"`
	Exchange        string `mapstructure:"exchange"`
	ExchangeType    string `mapstructure:"exchange_type"`
	RoutingKey      string `mapstructure:"routing_key"`
	Queue           string `mapstructure:"queue"`
	RetryQueue      string `mapstructure:"retry_queue"`
	DeadLetterQueue string `mapstructure:"dead_letter_queue"`
	PrefetchCount   int    `mapstructure:"prefetch_count"`
	ConsumerTag     string `mapstructure:"consumer_tag"`
}

// Init 初始化配置加载，从指定的路径读取 YAML 文件并解析到 Conf 变量中。
func Init(configPath string) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("读取配置文件失败: %w", err))
	}

	if err := viper.Unmarshal(&Conf); err != nil {
		panic(fmt.Errorf("无法将配置解析到结构体中: %w", err))
	}
}
