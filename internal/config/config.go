package config

import (
	"fmt"
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
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"` // debug, release
}

type DatabaseConfig struct {
	Driver   string `mapstructure:"driver"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Name     string `mapstructure:"name"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	SSLMode  string `mapstructure:"sslmode"`
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

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "debug")

	// Database
	v.SetDefault("database.driver", "postgres")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.name", "bellkeeper")
	v.SetDefault("database.user", "bellkeeper")
	v.SetDefault("database.password", "")
	v.SetDefault("database.sslmode", "disable")

	// RagFlow
	v.SetDefault("ragflow.base_url", "http://ragflow:9380")
	v.SetDefault("ragflow.timeout", 30)

	// N8N
	v.SetDefault("n8n.webhook_base_url", "http://n8n:5678")

	// Logging
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")

	// Features
	v.SetDefault("features.auto_parse", true)
	v.SetDefault("features.url_dedup", true)
	v.SetDefault("features.ai_summary", false)
}
