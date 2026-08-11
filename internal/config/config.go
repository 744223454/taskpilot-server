package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Name string `yaml:"Name"`
	Host string `yaml:"Host"`
	Port int    `yaml:"Port"`
	Mode string `yaml:"Mode"`

	HTTP struct {
		TrustedProxies []string `yaml:"TrustedProxies"`
	} `yaml:"HTTP"`

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
		AccessSecret       string `yaml:"AccessSecret"`
		AccessExpire       int64  `yaml:"AccessExpire"`
		RefreshExpire      int64  `yaml:"RefreshExpire"`
		CookieSecure       bool   `yaml:"CookieSecure"`
		LoginRateLimit     int64  `yaml:"LoginRateLimit"`
		LoginRateWindow    int64  `yaml:"LoginRateWindow"`
		RegisterRateLimit  int64  `yaml:"RegisterRateLimit"`
		RegisterRateWindow int64  `yaml:"RegisterRateWindow"`
	} `yaml:"Auth"`

	// CORS holds cross-origin resource sharing settings for the API.
	CORS struct {
		AllowedOrigins []string `yaml:"AllowedOrigins"`
	} `yaml:"CORS"`

	AI struct {
		BaseURL         string `yaml:"BaseURL"`
		APIKey          string `yaml:"APIKey"`
		Model           string `yaml:"Model"`
		RequestTimeout  int64  `yaml:"RequestTimeout"`
		MaxOutputTokens int64  `yaml:"MaxOutputTokens"`
	} `yaml:"AI"`

	Upload struct {
		Root                     string `yaml:"Root"`
		MaxFileBytes             int64  `yaml:"MaxFileBytes"`
		MaxPages                 int    `yaml:"MaxPages"`
		MaxTextChars             int    `yaml:"MaxTextChars"`
		MinEffectiveChars        int    `yaml:"MinEffectiveChars"`
		ExtractTimeout           int64  `yaml:"ExtractTimeout"`
		MaxConcurrentExtractions int    `yaml:"MaxConcurrentExtractions"`
		SlotWaitTimeout          int64  `yaml:"SlotWaitTimeout"`
		TempGrace                int64  `yaml:"TempGrace"`
		OrphanGrace              int64  `yaml:"OrphanGrace"`
		CleanupInterval          int64  `yaml:"CleanupInterval"`
	} `yaml:"Upload"`

	Worker struct {
		StreamKey         string `yaml:"StreamKey"`
		ConsumerGroup     string `yaml:"ConsumerGroup"`
		Concurrency       int    `yaml:"Concurrency"`
		BlockTimeout      int64  `yaml:"BlockTimeout"`
		ReconcileInterval int64  `yaml:"ReconcileInterval"`
		PendingGrace      int64  `yaml:"PendingGrace"`
		LeaseTimeout      int64  `yaml:"LeaseTimeout"`
		MaxRecoveries     int32  `yaml:"MaxRecoveries"`
		StreamRetention   int64  `yaml:"StreamRetention"`
		HeartbeatInterval int64  `yaml:"HeartbeatInterval"`
		HeartbeatTTL      int64  `yaml:"HeartbeatTTL"`
		ShutdownGrace     int64  `yaml:"ShutdownGrace"`
	} `yaml:"Worker"`
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
	if cfg.Auth.AccessExpire <= 0 {
		return cfg, fmt.Errorf("config Auth.AccessExpire must be positive")
	}
	if cfg.Auth.RefreshExpire == 0 {
		cfg.Auth.RefreshExpire = 30 * 24 * 60 * 60
	}
	if cfg.Auth.RefreshExpire < 0 {
		return cfg, fmt.Errorf("config Auth.RefreshExpire must not be negative")
	}
	if err := applyAuthRateLimitDefaults(&cfg); err != nil {
		return cfg, err
	}

	if len(cfg.CORS.AllowedOrigins) == 0 {
		cfg.CORS.AllowedOrigins = defaultCORSOrigins()
	}
	if cfg.AI.BaseURL == "" {
		cfg.AI.BaseURL = "https://ai.soruxgpt.com/v1"
	}
	if cfg.AI.Model == "" {
		cfg.AI.Model = "gpt-5.4"
	}
	if cfg.AI.RequestTimeout == 0 {
		cfg.AI.RequestTimeout = 180
	}
	if cfg.AI.RequestTimeout < 0 {
		return cfg, fmt.Errorf("config AI.RequestTimeout must not be negative")
	}
	if cfg.AI.MaxOutputTokens == 0 {
		cfg.AI.MaxOutputTokens = 8000
	}
	if cfg.AI.MaxOutputTokens < 0 {
		return cfg, fmt.Errorf("config AI.MaxOutputTokens must not be negative")
	}
	if err := applyUploadDefaults(&cfg); err != nil {
		return cfg, err
	}
	if cfg.Worker.StreamKey == "" {
		cfg.Worker.StreamKey = "taskpilot:queue:parse_jobs"
	}
	if cfg.Worker.ConsumerGroup == "" {
		cfg.Worker.ConsumerGroup = "taskpilot:parse_workers"
	}
	if cfg.Worker.Concurrency == 0 {
		cfg.Worker.Concurrency = 2
	}
	if cfg.Worker.Concurrency < 1 || cfg.Worker.Concurrency > 16 {
		return cfg, fmt.Errorf("config Worker.Concurrency must be between 1 and 16")
	}
	if err := applyWorkerDefaults(&cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func applyAuthRateLimitDefaults(cfg *Config) error {
	settings := []struct {
		name         string
		value        *int64
		defaultValue int64
	}{
		{"LoginRateLimit", &cfg.Auth.LoginRateLimit, 10},
		{"LoginRateWindow", &cfg.Auth.LoginRateWindow, 5 * 60},
		{"RegisterRateLimit", &cfg.Auth.RegisterRateLimit, 20},
		{"RegisterRateWindow", &cfg.Auth.RegisterRateWindow, 60 * 60},
	}
	for _, setting := range settings {
		if *setting.value < 0 {
			return fmt.Errorf("config Auth.%s must not be negative", setting.name)
		}
		if *setting.value == 0 {
			*setting.value = setting.defaultValue
		}
	}
	return nil
}

func applyUploadDefaults(cfg *Config) error {
	if cfg.Upload.Root == "" {
		cfg.Upload.Root = "uploads"
	}
	positiveSettings := []struct {
		name         string
		value        *int64
		defaultValue int64
	}{
		{"MaxFileBytes", &cfg.Upload.MaxFileBytes, 10 << 20},
		{"ExtractTimeout", &cfg.Upload.ExtractTimeout, 15},
		{"SlotWaitTimeout", &cfg.Upload.SlotWaitTimeout, 3},
		{"TempGrace", &cfg.Upload.TempGrace, 60 * 60},
		{"OrphanGrace", &cfg.Upload.OrphanGrace, 24 * 60 * 60},
		{"CleanupInterval", &cfg.Upload.CleanupInterval, 24 * 60 * 60},
	}
	for _, setting := range positiveSettings {
		if *setting.value < 0 {
			return fmt.Errorf("config Upload.%s must not be negative", setting.name)
		}
		if *setting.value == 0 {
			*setting.value = setting.defaultValue
		}
	}
	if cfg.Upload.MaxPages == 0 {
		cfg.Upload.MaxPages = 50
	}
	if cfg.Upload.MaxTextChars == 0 {
		cfg.Upload.MaxTextChars = 50000
	}
	if cfg.Upload.MinEffectiveChars == 0 {
		cfg.Upload.MinEffectiveChars = 20
	}
	if cfg.Upload.MaxPages < 1 || cfg.Upload.MaxTextChars < 1 || cfg.Upload.MinEffectiveChars < 1 {
		return fmt.Errorf("config Upload document limits must be positive")
	}
	if cfg.Upload.MaxConcurrentExtractions == 0 {
		cfg.Upload.MaxConcurrentExtractions = 2
	}
	if cfg.Upload.MaxConcurrentExtractions < 1 || cfg.Upload.MaxConcurrentExtractions > 4 {
		return fmt.Errorf("config Upload.MaxConcurrentExtractions must be between 1 and 4")
	}
	return nil
}

func applyWorkerDefaults(cfg *Config) error {
	workerDurations := []struct {
		name         string
		value        *int64
		defaultValue int64
	}{
		{"BlockTimeout", &cfg.Worker.BlockTimeout, 5},
		{"ReconcileInterval", &cfg.Worker.ReconcileInterval, 30},
		{"PendingGrace", &cfg.Worker.PendingGrace, 30},
		{"LeaseTimeout", &cfg.Worker.LeaseTimeout, 600},
		{"StreamRetention", &cfg.Worker.StreamRetention, 7 * 24 * 60 * 60},
		{"HeartbeatInterval", &cfg.Worker.HeartbeatInterval, 10},
		{"HeartbeatTTL", &cfg.Worker.HeartbeatTTL, 30},
		{"ShutdownGrace", &cfg.Worker.ShutdownGrace, 180},
	}
	for _, setting := range workerDurations {
		if *setting.value < 0 {
			return fmt.Errorf("config Worker.%s must not be negative", setting.name)
		}
		if *setting.value == 0 {
			*setting.value = setting.defaultValue
		}
	}
	if cfg.Worker.MaxRecoveries < 0 {
		return fmt.Errorf("config Worker.MaxRecoveries must not be negative")
	}
	if cfg.Worker.MaxRecoveries == 0 {
		cfg.Worker.MaxRecoveries = 3
	}
	if cfg.Worker.HeartbeatTTL <= cfg.Worker.HeartbeatInterval {
		return fmt.Errorf("config Worker.HeartbeatTTL must exceed HeartbeatInterval")
	}
	return nil
}

// defaultCORSOrigins returns the frontend origins allowed to call the API.
// Local development runs on Vite (localhost:5173/5174); the deployed frontend
// and API share the canonical taskpilot.1kuansi.cn origin.
func defaultCORSOrigins() []string {
	return []string{
		"http://localhost:5173",
		"http://localhost:5174",
		"https://taskpilot.1kuansi.cn",
	}
}

func applyEnvOverrides(cfg *Config) {
	cfg.Name = envString("TASKPILOT_NAME", cfg.Name)
	cfg.Host = envString("TASKPILOT_HOST", cfg.Host)
	cfg.Port = envInt("TASKPILOT_PORT", cfg.Port)
	cfg.Mode = envString("TASKPILOT_MODE", cfg.Mode)
	cfg.HTTP.TrustedProxies = envStringSlice("TASKPILOT_HTTP_TRUSTED_PROXIES", cfg.HTTP.TrustedProxies)

	cfg.Database.DataSource = envString("TASKPILOT_DATABASE_DSN", cfg.Database.DataSource)

	cfg.Cache.Host = envString("TASKPILOT_REDIS_HOST", cfg.Cache.Host)
	cfg.Cache.Pass = envString("TASKPILOT_REDIS_PASS", cfg.Cache.Pass)
	cfg.Cache.Type = envString("TASKPILOT_REDIS_TYPE", cfg.Cache.Type)

	cfg.Auth.AccessSecret = envString("TASKPILOT_AUTH_ACCESS_SECRET", cfg.Auth.AccessSecret)
	cfg.Auth.AccessExpire = envInt64("TASKPILOT_AUTH_ACCESS_EXPIRE", cfg.Auth.AccessExpire)
	cfg.Auth.RefreshExpire = envInt64("TASKPILOT_AUTH_REFRESH_EXPIRE", cfg.Auth.RefreshExpire)
	cfg.Auth.CookieSecure = envBool("TASKPILOT_AUTH_COOKIE_SECURE", cfg.Auth.CookieSecure)
	cfg.Auth.LoginRateLimit = envInt64("TASKPILOT_AUTH_LOGIN_RATE_LIMIT", cfg.Auth.LoginRateLimit)
	cfg.Auth.LoginRateWindow = envInt64("TASKPILOT_AUTH_LOGIN_RATE_WINDOW", cfg.Auth.LoginRateWindow)
	cfg.Auth.RegisterRateLimit = envInt64("TASKPILOT_AUTH_REGISTER_RATE_LIMIT", cfg.Auth.RegisterRateLimit)
	cfg.Auth.RegisterRateWindow = envInt64("TASKPILOT_AUTH_REGISTER_RATE_WINDOW", cfg.Auth.RegisterRateWindow)

	cfg.CORS.AllowedOrigins = envStringSlice("TASKPILOT_CORS_ALLOWED_ORIGINS", cfg.CORS.AllowedOrigins)

	cfg.AI.BaseURL = envString("TASKPILOT_AI_BASE_URL", cfg.AI.BaseURL)
	cfg.AI.APIKey = envString("TASKPILOT_AI_API_KEY", cfg.AI.APIKey)
	cfg.AI.Model = envString("TASKPILOT_AI_MODEL", cfg.AI.Model)
	cfg.AI.RequestTimeout = envInt64("TASKPILOT_AI_REQUEST_TIMEOUT", cfg.AI.RequestTimeout)
	cfg.AI.MaxOutputTokens = envInt64("TASKPILOT_AI_MAX_OUTPUT_TOKENS", cfg.AI.MaxOutputTokens)

	cfg.Upload.Root = envString("TASKPILOT_UPLOAD_ROOT", cfg.Upload.Root)
	cfg.Upload.MaxFileBytes = envInt64("TASKPILOT_UPLOAD_MAX_FILE_BYTES", cfg.Upload.MaxFileBytes)
	cfg.Upload.MaxPages = envInt("TASKPILOT_UPLOAD_MAX_PAGES", cfg.Upload.MaxPages)
	cfg.Upload.MaxTextChars = envInt("TASKPILOT_UPLOAD_MAX_TEXT_CHARS", cfg.Upload.MaxTextChars)
	cfg.Upload.MinEffectiveChars = envInt("TASKPILOT_UPLOAD_MIN_EFFECTIVE_CHARS", cfg.Upload.MinEffectiveChars)
	cfg.Upload.ExtractTimeout = envInt64("TASKPILOT_UPLOAD_EXTRACT_TIMEOUT", cfg.Upload.ExtractTimeout)
	cfg.Upload.MaxConcurrentExtractions = envInt("TASKPILOT_UPLOAD_MAX_CONCURRENT_EXTRACTIONS", cfg.Upload.MaxConcurrentExtractions)
	cfg.Upload.SlotWaitTimeout = envInt64("TASKPILOT_UPLOAD_SLOT_WAIT_TIMEOUT", cfg.Upload.SlotWaitTimeout)
	cfg.Upload.TempGrace = envInt64("TASKPILOT_UPLOAD_TEMP_GRACE", cfg.Upload.TempGrace)
	cfg.Upload.OrphanGrace = envInt64("TASKPILOT_UPLOAD_ORPHAN_GRACE", cfg.Upload.OrphanGrace)
	cfg.Upload.CleanupInterval = envInt64("TASKPILOT_UPLOAD_CLEANUP_INTERVAL", cfg.Upload.CleanupInterval)

	cfg.Worker.StreamKey = envString("TASKPILOT_WORKER_STREAM_KEY", cfg.Worker.StreamKey)
	cfg.Worker.ConsumerGroup = envString("TASKPILOT_WORKER_CONSUMER_GROUP", cfg.Worker.ConsumerGroup)
	cfg.Worker.Concurrency = envInt("TASKPILOT_WORKER_CONCURRENCY", cfg.Worker.Concurrency)
	cfg.Worker.BlockTimeout = envInt64("TASKPILOT_WORKER_BLOCK_TIMEOUT", cfg.Worker.BlockTimeout)
	cfg.Worker.ReconcileInterval = envInt64("TASKPILOT_WORKER_RECONCILE_INTERVAL", cfg.Worker.ReconcileInterval)
	cfg.Worker.PendingGrace = envInt64("TASKPILOT_WORKER_PENDING_GRACE", cfg.Worker.PendingGrace)
	cfg.Worker.LeaseTimeout = envInt64("TASKPILOT_WORKER_LEASE_TIMEOUT", cfg.Worker.LeaseTimeout)
	cfg.Worker.MaxRecoveries = int32(envInt64("TASKPILOT_WORKER_MAX_RECOVERIES", int64(cfg.Worker.MaxRecoveries)))
	cfg.Worker.StreamRetention = envInt64("TASKPILOT_WORKER_STREAM_RETENTION", cfg.Worker.StreamRetention)
	cfg.Worker.HeartbeatInterval = envInt64("TASKPILOT_WORKER_HEARTBEAT_INTERVAL", cfg.Worker.HeartbeatInterval)
	cfg.Worker.HeartbeatTTL = envInt64("TASKPILOT_WORKER_HEARTBEAT_TTL", cfg.Worker.HeartbeatTTL)
	cfg.Worker.ShutdownGrace = envInt64("TASKPILOT_WORKER_SHUTDOWN_GRACE", cfg.Worker.ShutdownGrace)
}

func envBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
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

// envStringSlice reads a comma-separated list from the environment, e.g.
// "http://a,https://b". An empty value falls back to the configured list.
func envStringSlice(key string, fallback []string) []string {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
