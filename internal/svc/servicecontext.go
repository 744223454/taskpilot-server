package svc

import (
	"log/slog"
	"os"

	"github.com/744223454/taskpilot-server/internal/config"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	"github.com/744223454/taskpilot-server/pkg/database"
	"gorm.io/gorm"
)

type ServiceContext struct {
	Config config.Config
	DB     *gorm.DB
	JWT    *jwtauth.Manager
	Logger *slog.Logger
}

func NewServiceContext(c config.Config) *ServiceContext {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	db, err := database.NewPostgres(c.Database.DataSource)
	if err != nil {
		logger.Warn("database initialization failed", "error", err)
		db = nil
	}

	return &ServiceContext{
		Config: c,
		DB:     db,
		JWT:    jwtauth.NewManager(c.Auth.AccessSecret, c.Auth.AccessExpire),
		Logger: logger,
	}
}
