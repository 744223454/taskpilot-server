package svc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/744223454/taskpilot-server/internal/config"
	"github.com/744223454/taskpilot-server/pkg/ai"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	cachepkg "github.com/744223454/taskpilot-server/pkg/cache"
	"github.com/744223454/taskpilot-server/pkg/database"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type ServiceContext struct {
	Config          config.Config
	DB              *gorm.DB
	JWT             *jwtauth.Manager
	Redis           *redis.Client
	RefreshSessions jwtauth.RefreshSessionStore
	ParseJobs       cachepkg.ParseJobQueue
	Parser          ai.Parser
	Logger          *slog.Logger
}

func NewServiceContext(c config.Config) *ServiceContext {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	db, err := database.NewPostgres(c.Database.DataSource)
	if err != nil {
		logger.Warn("database initialization failed", "error", err)
		db = nil
	}

	serverContext := &ServiceContext{
		Config: c,
		DB:     db,
		JWT:    jwtauth.NewManager(c.Auth.AccessSecret, c.Auth.AccessExpire),
		Logger: logger,
	}
	if c.Cache.Host == "" {
		logger.Warn("redis initialization skipped", "error", "config Cache.Host is empty")
		return serverContext
	}

	redisClient := cachepkg.NewRedis(c.Cache.Host, c.Cache.Pass)
	serverContext.Redis = redisClient
	serverContext.RefreshSessions = cachepkg.NewRefreshSessionStore(redisClient)
	serverContext.ParseJobs = cachepkg.NewParseJobQueue(redisClient, c.Worker.StreamKey, c.Worker.ConsumerGroup)
	pingContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := redisClient.Ping(pingContext).Err(); err != nil {
		logger.Warn("redis initialization failed", "error", err)
	}
	return serverContext
}

func (s *ServiceContext) Close() error {
	if s == nil {
		return nil
	}

	var closeErrors []error
	if s.Redis != nil {
		if err := s.Redis.Close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close redis client: %w", err))
		}
	}
	if s.DB != nil {
		sqlDB, err := s.DB.DB()
		if err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("get postgres connection pool: %w", err))
		} else if err := sqlDB.Close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close postgres connection pool: %w", err))
		}
	}
	return errors.Join(closeErrors...)
}
