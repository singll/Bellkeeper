package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	RagFlow  RagFlowConfig  `mapstructure:"ragflow"`
	N8N      N8NConfig      `mapstructure:"n8n"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	Features FeatureConfig  `mapstructure:"features"`
	LLMProxy LLMProxyConfig `mapstructure:"llm_proxy"`
	Classify ClassifyConfig `mapstructure:"classify"`
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
	Name      string   `mapstructure:"name"`
	BaseURL   string   `mapstructure:"base_url"`
	APIKey    string   `mapstructure:"api_key"`
	RPM       int      `mapstructure:"rpm"`
	RPD       int      `mapstructure:"rpd"`
	Priority  int      `mapstructure:"priority"`
	Models    []string `mapstructure:"models"`
	IsEnabled bool     `mapstructure:"is_enabled"`
	IsFree    bool     `mapstructure:"is_free"`
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

type RagFlowConfig struct {
	BaseURL string `mapstructure:"base_url"`
	APIKey  string `mapstructure:"api_key"`
	Timeout int    `mapstructure:"timeout"`
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
	AISummary bool `mapstructure:"ai_summary"`
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
		cfg.LLMProxy.Channels[i].BaseURL = os.ExpandEnv(cfg.LLMProxy.Channels[i].BaseURL)
		cfg.LLMProxy.Channels[i].APIKey = os.ExpandEnv(cfg.LLMProxy.Channels[i].APIKey)
	}

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

	// RagFlow
	v.SetDefault("ragflow.base_url", "http://ragflow:9380")
	v.SetDefault("ragflow.timeout", 30)

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
	v.SetDefault("features.ai_summary", false)

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

	// Classify
	v.SetDefault("classify.enabled", false)
	v.SetDefault("classify.llm_proxy_url", "http://localhost:8080/api/llm/v1")
	v.SetDefault("classify.model", "glm-4-flash")
	v.SetDefault("classify.temperature", 0.3)
	v.SetDefault("classify.max_content_len", 1000)
	v.SetDefault("classify.timeout", 5)
	v.SetDefault("classify.prompt", "")
}
