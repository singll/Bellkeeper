package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	N8N           N8NConfig           `mapstructure:"n8n"`
	Logging       LoggingConfig       `mapstructure:"logging"`
	Features      FeatureConfig       `mapstructure:"features"`
	LLMProxy      LLMProxyConfig      `mapstructure:"llm_proxy"`
	LLMJobQueue   LLMJobQueueConfig   `mapstructure:"llm_job_queue"`
	Classify      ClassifyConfig      `mapstructure:"classify"`
	FileIngestion FileIngestionConfig `mapstructure:"file_ingestion"`
	RSSFetcher    RSSFetcherConfig    `mapstructure:"rss_fetcher"`
	Matrix        MatrixConfig        `mapstructure:"matrix"`
	Redis         RedisConfig         `mapstructure:"redis"`
	NATS          NATSConfig          `mapstructure:"nats"`
	Memos         MemosConfig         `mapstructure:"memos"`
	Meilisearch   MeilisearchConfig   `mapstructure:"meilisearch"`
	Knowledge     KnowledgeConfig     `mapstructure:"knowledge"`
	CrawlQueue    CrawlQueueConfig    `mapstructure:"crawl_queue"`
}

type LLMProxyConfig struct {
	Enabled           bool                 `mapstructure:"enabled"`
	DefaultTimeout    int                  `mapstructure:"default_timeout"`
	MaxRetries        int                  `mapstructure:"max_retries"`
	MaxWaitSeconds    int                  `mapstructure:"max_wait_seconds"`
	BackoffCapSeconds int                  `mapstructure:"backoff_cap_seconds"`
	BackoffJitter     float64              `mapstructure:"backoff_jitter"`
	DefaultBucketRPM  int                  `mapstructure:"default_bucket_rpm"`
	CircuitBreaker    CircuitBreakerConfig `mapstructure:"circuit_breaker"`
	ModelGroups       []ModelGroupConfig   `mapstructure:"model_groups"`
	Channels          []ChannelConfig      `mapstructure:"channels"`
	// Balance monitoring
	BalanceSyncInterval int `mapstructure:"balance_sync_interval"` // seconds, 0 = disabled
	LogRetentionDays    int `mapstructure:"log_retention_days"`    // 0 = default 30
	// Coding routing strategy
	CodingRoutingStrategy string           `mapstructure:"coding_routing_strategy"` // free_first | quality_first | complexity_aware
	Complexity            ComplexityConfig `mapstructure:"complexity"`
}

type LLMJobQueueConfig struct {
	Enabled                 bool `mapstructure:"enabled"`
	Workers                 int  `mapstructure:"workers"`
	PollIntervalSeconds     int  `mapstructure:"poll_interval_seconds"`
	MaxRetries              int  `mapstructure:"max_retries"`
	InitialBackoffSeconds   int  `mapstructure:"initial_backoff_seconds"`
	MaxBackoffSeconds       int  `mapstructure:"max_backoff_seconds"`
	StaleTimeoutMinutes     int  `mapstructure:"stale_timeout_minutes"`
	RecoveryIntervalMinutes int  `mapstructure:"recovery_interval_minutes"`
	JobTimeoutSeconds       int  `mapstructure:"job_timeout_seconds"`
}

type ComplexityConfig struct {
	SimpleThresholdTokens  int      `mapstructure:"simple_threshold_tokens"`
	ComplexThresholdTokens int      `mapstructure:"complex_threshold_tokens"`
	ComplexKeywords        []string `mapstructure:"complex_keywords"`
}

type CircuitBreakerConfig struct {
	FailureThreshold   int `mapstructure:"failure_threshold"`
	CooldownSeconds    int `mapstructure:"cooldown_seconds"`
	HalfOpenMax        int `mapstructure:"half_open_max"`
	ErrorWindowSeconds int `mapstructure:"error_window_seconds"`
}

type ModelGroupConfig struct {
	Name             string             `mapstructure:"name"`
	Description      string             `mapstructure:"description"`
	Strategy         string             `mapstructure:"strategy"`
	StickyTTLSeconds int                `mapstructure:"sticky_ttl_seconds"`
	Members          []ModelGroupMember `mapstructure:"members"`
}

type ModelGroupMember struct {
	Channel string `mapstructure:"channel"`
	Model   string `mapstructure:"model"`
	Weight  int    `mapstructure:"weight"`
}

type ChannelConfig struct {
	ID                  uint     `mapstructure:"-"` // DB row id (0 for YAML-only config); used for rate-limit learning + bindings
	Name                string   `mapstructure:"name"`
	BaseURL             string   `mapstructure:"base_url"`
	APIKey              string   `mapstructure:"api_key"`
	ProviderType        string   `mapstructure:"provider_type"`
	RawAPIKey           string   `mapstructure:"-"` // Pre-expansion value, set by Load()
	RPM                 int      `mapstructure:"rpm"`
	BalanceProviderType string   `mapstructure:"balance_provider_type"`
	BalanceConfigJSON   string   `mapstructure:"balance_config_json"`
	ModelRPMOverrides   string   `mapstructure:"model_rpm_overrides"`
	RPD                 int      `mapstructure:"rpd"`
	Priority            int      `mapstructure:"priority"`
	Models              []string `mapstructure:"models"`
	IsEnabled           bool     `mapstructure:"is_enabled"`
	IsFree              bool     `mapstructure:"is_free"`
	TaskTypes           []string `mapstructure:"task_types"` // empty = eligible for all task types
	Tier                string   `mapstructure:"tier"`       // free | standard | premium; empty = derived from IsFree
}

type ServerConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	Mode            string `mapstructure:"mode"`             // debug, release
	APIKey          string `mapstructure:"api_key"`          // API Key for internal service auth
	ShutdownTimeout int    `mapstructure:"shutdown_timeout"` // graceful shutdown timeout in seconds
}

type DatabaseConfig struct {
	Driver          string `mapstructure:"driver"`
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	Name            string `mapstructure:"name"`
	User            string `mapstructure:"user"`
	Password        string `mapstructure:"password"`
	SSLMode         string `mapstructure:"sslmode"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"` // minutes
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode)
}

type N8NConfig struct {
	WebhookBaseURL string `mapstructure:"webhook_base_url"`
	APIBaseURL     string `mapstructure:"api_base_url"`
	APIKey         string `mapstructure:"api_key"`
	Timeout        int    `mapstructure:"timeout"` // HTTP client timeout in seconds
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

type FeatureConfig struct {
	AutoParse bool `mapstructure:"auto_parse"`
	URLDedup  bool `mapstructure:"url_dedup"`
}

type ClassifyConfig struct {
	Enabled       bool    `mapstructure:"enabled"`
	LLMProxyURL   string  `mapstructure:"llm_proxy_url"`
	Model         string  `mapstructure:"model"`
	Temperature   float64 `mapstructure:"temperature"`
	MaxContentLen int     `mapstructure:"max_content_len"`
	Timeout       int     `mapstructure:"timeout"` // HTTP client timeout in seconds
	Prompt        string  `mapstructure:"prompt"`  // empty = use built-in default
}

type FileIngestionConfig struct {
	Enabled      bool              `mapstructure:"enabled"`
	BasePath     string            `mapstructure:"base_path"`
	RawDir       string            `mapstructure:"raw_dir"`
	WorkingDir   string            `mapstructure:"working_dir"`
	DefaultLayer string            `mapstructure:"default_layer"`
	Trafilatura  TrafilaturaConfig `mapstructure:"trafilatura"`
	Firecrawl    FirecrawlConfig   `mapstructure:"firecrawl"`
}

type TrafilaturaConfig struct {
	Enabled          bool `mapstructure:"enabled"`
	Timeout          int  `mapstructure:"timeout"`
	MinContentLength int  `mapstructure:"min_content_length"`
}

type FirecrawlConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	APIURL       string `mapstructure:"api_url"`
	Timeout      int    `mapstructure:"timeout"`
	FallbackOnly bool   `mapstructure:"fallback_only"`
}

type MatrixConfig struct {
	HomeserverURL  string   `mapstructure:"homeserver_url"`
	BotUserID      string   `mapstructure:"bot_user_id"`
	BotAccessToken string   `mapstructure:"bot_access_token"`
	DeviceID       string   `mapstructure:"device_id"`
	SyncTimeout    int      `mapstructure:"sync_timeout"`   // milliseconds
	CommandPrefix  string   `mapstructure:"command_prefix"` // comma-separated prefixes
	MaxRetry       int      `mapstructure:"max_retry"`
	AdminUsers     []string `mapstructure:"admin_users"` // list of admin user IDs
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	DB       int    `mapstructure:"db"`
	Password string `mapstructure:"password"`
}

type NATSConfig struct {
	URL     string            `mapstructure:"url"`
	Streams NATSStreamsConfig `mapstructure:"streams"`
}

type NATSStreamsConfig struct {
	Notifications string `mapstructure:"notifications"`
	Commands      string `mapstructure:"commands"`
}

type MemosConfig struct {
	BaseURL  string `mapstructure:"base_url"`
	APIToken string `mapstructure:"api_token"`
	Enabled  bool   `mapstructure:"enabled"`
}

// MeilisearchConfig Meilisearch 搜索服务配置
type MeilisearchConfig struct {
	URL    string `mapstructure:"url"`
	APIKey string `mapstructure:"api_key"`
	Index  string `mapstructure:"index"`
}

// KnowledgeConfig 知识库索引配置
type KnowledgeConfig struct {
	Enabled      bool            `mapstructure:"enabled"`
	BasePath     string          `mapstructure:"base_path"`
	ScanDirs     []ScanDirConfig `mapstructure:"scan_dirs"`
	ScanInterval int             `mapstructure:"scan_interval"`
	ChunkMinSize int             `mapstructure:"chunk_min_size"`
	ChunkMaxSize int             `mapstructure:"chunk_max_size"`
}

// RSSFetcherConfig RSS 抓取器配置
type RSSFetcherConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	CheckInterval int    `mapstructure:"check_interval"`  // 轮询间隔（秒）
	MaxPerBatch   int    `mapstructure:"max_per_batch"`   // 每批最大处理 feed 数
	Timeout       int    `mapstructure:"timeout"`         // 单个 feed 超时（秒）
	RSSHubBaseURL string `mapstructure:"rsshub_base_url"` // RSSHub 实例地址，用于拼接以 / 开头的相对路径
}

// CrawlQueueConfig 爬取队列配置
type CrawlQueueConfig struct {
	Enabled                 bool     `mapstructure:"enabled"`
	FirecrawlWorkers        int      `mapstructure:"firecrawl_workers"`
	TrafilaturaWorkers      int      `mapstructure:"trafilatura_workers"`
	AutoWorkers             int      `mapstructure:"auto_workers"`
	PollInterval            int      `mapstructure:"poll_interval"` // 秒
	MaxRetries              int      `mapstructure:"max_retries"`
	RetryBackoffBase        int      `mapstructure:"retry_backoff_base"` // 秒
	RetryBackoffMax         int      `mapstructure:"retry_backoff_max"`  // 秒
	DeadLetterThreshold     int      `mapstructure:"dead_letter_threshold"`
	PaywallThreshold        int      `mapstructure:"paywall_threshold"`         // 连续空内容次数
	BlockedDomains          []string `mapstructure:"blocked_domains"`           // 预设付费墙域名
	StaleTimeoutMinutes     int      `mapstructure:"stale_timeout_minutes"`     // 运行超过此分钟数视为卡死
	RecoveryIntervalMinutes int      `mapstructure:"recovery_interval_minutes"` // 卡死回收检查间隔（分钟）
}

// ScanDirConfig 扫描目录配置
type ScanDirConfig struct {
	Path  string `mapstructure:"path"`
	Layer string `mapstructure:"layer"`
}

func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Config file
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("bellkeeper")
		v.SetConfigType("yaml")
		v.AddConfigPath("./config")
		v.AddConfigPath("/etc/bellkeeper")
		v.AddConfigPath("$HOME/.bellkeeper")
	}

	// Environment variables
	v.SetEnvPrefix("BELLKEEPER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found, use defaults and env vars
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Expand ${VAR} references in LLM proxy channel configs
	for i := range cfg.LLMProxy.Channels {
		cfg.LLMProxy.Channels[i].RawAPIKey = cfg.LLMProxy.Channels[i].APIKey
		cfg.LLMProxy.Channels[i].BaseURL = os.ExpandEnv(cfg.LLMProxy.Channels[i].BaseURL)
		cfg.LLMProxy.Channels[i].APIKey = os.ExpandEnv(cfg.LLMProxy.Channels[i].APIKey)
	}

	// Expand ${VAR} references in Matrix config
	cfg.Matrix.HomeserverURL = os.ExpandEnv(cfg.Matrix.HomeserverURL)
	cfg.Matrix.BotAccessToken = os.ExpandEnv(cfg.Matrix.BotAccessToken)

	// Expand ${VAR} references in Firecrawl config
	cfg.FileIngestion.Firecrawl.APIURL = os.ExpandEnv(cfg.FileIngestion.Firecrawl.APIURL)

	// Expand ${VAR} references in Memos config
	cfg.Memos.BaseURL = os.ExpandEnv(cfg.Memos.BaseURL)
	cfg.Memos.APIToken = os.ExpandEnv(cfg.Memos.APIToken)

	// Expand ${VAR} references in Meilisearch config
	cfg.Meilisearch.APIKey = os.ExpandEnv(cfg.Meilisearch.APIKey)

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "debug")
	v.SetDefault("server.api_key", "")
	v.SetDefault("server.shutdown_timeout", 10)

	// Database
	v.SetDefault("database.driver", "postgres")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.name", "bellkeeper")
	v.SetDefault("database.user", "bellkeeper")
	v.SetDefault("database.password", "")
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("database.max_open_conns", 100)
	v.SetDefault("database.conn_max_lifetime", 60)

	// N8N — URL defaults left empty so workflow.go can fall back to DB settings
	v.SetDefault("n8n.webhook_base_url", "")
	v.SetDefault("n8n.api_base_url", "")
	v.SetDefault("n8n.api_key", "")
	v.SetDefault("n8n.timeout", 30)

	// Logging
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")

	// Features
	v.SetDefault("features.auto_parse", true)
	v.SetDefault("features.url_dedup", true)

	// LLM Proxy
	v.SetDefault("llm_proxy.enabled", false)
	v.SetDefault("llm_proxy.default_timeout", 60)
	v.SetDefault("llm_proxy.max_retries", 3)
	v.SetDefault("llm_proxy.max_wait_seconds", 30)
	v.SetDefault("llm_proxy.backoff_cap_seconds", 60)
	v.SetDefault("llm_proxy.backoff_jitter", 0.5)
	v.SetDefault("llm_proxy.default_bucket_rpm", 1000)
	v.SetDefault("llm_proxy.circuit_breaker.failure_threshold", 5)
	v.SetDefault("llm_proxy.circuit_breaker.cooldown_seconds", 120)
	v.SetDefault("llm_proxy.circuit_breaker.half_open_max", 1)
	v.SetDefault("llm_proxy.circuit_breaker.error_window_seconds", 300)

	// LLM Job Queue
	v.SetDefault("llm_job_queue.enabled", true)
	v.SetDefault("llm_job_queue.workers", 1)
	v.SetDefault("llm_job_queue.poll_interval_seconds", 5)
	v.SetDefault("llm_job_queue.max_retries", 24)
	v.SetDefault("llm_job_queue.initial_backoff_seconds", 30)
	v.SetDefault("llm_job_queue.max_backoff_seconds", 900)
	v.SetDefault("llm_job_queue.stale_timeout_minutes", 30)
	v.SetDefault("llm_job_queue.recovery_interval_minutes", 5)
	v.SetDefault("llm_job_queue.job_timeout_seconds", 600)

	// Classify
	v.SetDefault("classify.enabled", false)
	v.SetDefault("classify.llm_proxy_url", "http://localhost:8080/api/llm/v1")
	v.SetDefault("classify.model", "glm-4-flash")
	v.SetDefault("classify.temperature", 0.3)
	v.SetDefault("classify.max_content_len", 1000)
	v.SetDefault("classify.timeout", 5)
	v.SetDefault("classify.prompt", "")

	// File Ingestion
	v.SetDefault("file_ingestion.enabled", false)
	v.SetDefault("file_ingestion.base_path", "/mnt/knowledge")
	v.SetDefault("file_ingestion.raw_dir", "raw")
	v.SetDefault("file_ingestion.working_dir", "working")
	v.SetDefault("file_ingestion.default_layer", "raw")
	v.SetDefault("file_ingestion.trafilatura.enabled", true)
	v.SetDefault("file_ingestion.trafilatura.timeout", 15)
	v.SetDefault("file_ingestion.trafilatura.min_content_length", 100)
	v.SetDefault("file_ingestion.firecrawl.enabled", true)
	v.SetDefault("file_ingestion.firecrawl.api_url", "")
	v.SetDefault("file_ingestion.firecrawl.timeout", 60)
	v.SetDefault("file_ingestion.firecrawl.fallback_only", true)

	// Matrix
	v.SetDefault("matrix.homeserver_url", "https://matrix.singll.net")
	v.SetDefault("matrix.bot_user_id", "@bellkeeper:matrix.singll.net")
	v.SetDefault("matrix.bot_access_token", "")
	v.SetDefault("matrix.device_id", "BELLKEEPER_KEEPER")
	v.SetDefault("matrix.sync_timeout", 30000)
	v.SetDefault("matrix.command_prefix", "!,！")
	v.SetDefault("matrix.max_retry", 3)
	v.SetDefault("matrix.admin_users", []string{"@singll:matrix.singll.net"})

	// Redis
	v.SetDefault("redis.host", "sp-redis")
	v.SetDefault("redis.port", 6379)
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.password", "")

	// NATS
	v.SetDefault("nats.url", "nats://sp-nats:4222")
	v.SetDefault("nats.streams.notifications", "notifications")
	v.SetDefault("nats.streams.commands", "commands")

	// Memos
	v.SetDefault("memos.enabled", false)
	v.SetDefault("memos.base_url", "")
	v.SetDefault("memos.api_token", "")

	// Meilisearch
	v.SetDefault("meilisearch.url", "http://sp-meilisearch:7700")
	v.SetDefault("meilisearch.api_key", "")
	v.SetDefault("meilisearch.index", "knowledge_chunks")

	// Knowledge
	v.SetDefault("knowledge.enabled", true)
	v.SetDefault("knowledge.base_path", "/mnt/knowledge")
	v.SetDefault("knowledge.scan_interval", 300)
	v.SetDefault("knowledge.chunk_min_size", 100)
	v.SetDefault("knowledge.chunk_max_size", 600)
	// 三层知识库：raw 绝不进 Meili（仅 ingest 落盘 + pkb-curate 待处理源）；
	// archive 中等价值归档、vault 高价值卡片均进 Meili（vault 另经 Obsidian LiveSync 下行本地）。
	// working/ 不在此列——它是 ReportService 的渠道消息存档区，不应进知识检索。
	v.SetDefault("knowledge.scan_dirs", []map[string]string{
		{"path": "archive", "layer": "archive"},
		{"path": "vault", "layer": "vault"},
	})

	// RSS Fetcher
	v.SetDefault("rss_fetcher.enabled", true)
	v.SetDefault("rss_fetcher.check_interval", 60)
	v.SetDefault("rss_fetcher.max_per_batch", 5)
	v.SetDefault("rss_fetcher.timeout", 30)
	v.SetDefault("rss_fetcher.rsshub_base_url", "")

	// Crawl Queue
	v.SetDefault("crawl_queue.enabled", true)
	v.SetDefault("crawl_queue.firecrawl_workers", 3)
	v.SetDefault("crawl_queue.trafilatura_workers", 2)
	v.SetDefault("crawl_queue.auto_workers", 2)
	v.SetDefault("crawl_queue.poll_interval", 5)
	v.SetDefault("crawl_queue.max_retries", 4)
	v.SetDefault("crawl_queue.retry_backoff_base", 60)
	v.SetDefault("crawl_queue.retry_backoff_max", 7200)
	v.SetDefault("crawl_queue.dead_letter_threshold", 6)
	v.SetDefault("crawl_queue.paywall_threshold", 2)
	v.SetDefault("crawl_queue.blocked_domains", []string{
		"wsj.com", "nytimes.com", "reuters.com", "bloomberg.com",
		"medium.com", "ft.com", "economist.com", "wired.com",
		"technologyreview.com", "scientificamerican.com",
	})
	v.SetDefault("crawl_queue.stale_timeout_minutes", 10)
	v.SetDefault("crawl_queue.recovery_interval_minutes", 5)
}
