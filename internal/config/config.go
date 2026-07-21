package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Name string `yaml:"Name"`
	Host string `yaml:"Host"`
	Port int    `yaml:"Port"`
	Mode string `yaml:"Mode"`

	// Database holds PostgreSQL connection settings.
	Database struct {
		DataSource string `yaml:"DataSource"`
	} `yaml:"Database"`

	// Cache holds Redis connection settings.
	Cache struct {
		Host string `yaml:"Host"`
		Pass string `yaml:"Pass"`
		Type string `yaml:"Type"`
	} `yaml:"Cache"`

	// Auth holds JWT signing settings.
	Auth struct {
		AccessSecret string `yaml:"AccessSecret"`
		AccessExpire int64  `yaml:"AccessExpire"`
	} `yaml:"Auth"`
}

func Load(path string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal config: %w", err)
	}

	applyEnvOverrides(&cfg)

	if cfg.Host == "" {
		cfg.Host = "0.0.0.0"
	}
	if cfg.Port == 0 {
		cfg.Port = 8888
	}
	if cfg.Mode == "" {
		cfg.Mode = "debug"
	}
	if cfg.Cache.Type == "" {
		cfg.Cache.Type = "node"
	}
	if cfg.Auth.AccessSecret == "" {
		return cfg, fmt.Errorf("config Auth.AccessSecret is required")
	}
	if cfg.Auth.AccessExpire == 0 {
		return cfg, fmt.Errorf("config Auth.AccessExpire is required")
	}

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	cfg.Name = envString("TASKPILOT_NAME", cfg.Name)
	cfg.Host = envString("TASKPILOT_HOST", cfg.Host)
	cfg.Port = envInt("TASKPILOT_PORT", cfg.Port)
	cfg.Mode = envString("TASKPILOT_MODE", cfg.Mode)

	cfg.Database.DataSource = envString("TASKPILOT_DATABASE_DSN", cfg.Database.DataSource)

	cfg.Cache.Host = envString("TASKPILOT_REDIS_HOST", cfg.Cache.Host)
	cfg.Cache.Pass = envString("TASKPILOT_REDIS_PASS", cfg.Cache.Pass)
	cfg.Cache.Type = envString("TASKPILOT_REDIS_TYPE", cfg.Cache.Type)

	cfg.Auth.AccessSecret = envString("TASKPILOT_AUTH_ACCESS_SECRET", cfg.Auth.AccessSecret)
	cfg.Auth.AccessExpire = envInt64("TASKPILOT_AUTH_ACCESS_EXPIRE", cfg.Auth.AccessExpire)
}

func envString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			return parsed
		}
	}
	return fallback
}
