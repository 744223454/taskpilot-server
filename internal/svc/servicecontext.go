package svc

import (
	"github.com/744223454/taskpilot-server/internal/config"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	"github.com/744223454/taskpilot-server/pkg/database"
	"gorm.io/gorm"
)

type ServiceContext struct {
	Config config.Config
	DB     *gorm.DB
	JWT    *jwtauth.Manager
}

func NewServiceContext(c config.Config) *ServiceContext {
	db, err := database.NewPostgres(c.Database.DataSource)
	if err != nil {
		// Keep the app bootable while persistence is still under construction.
		db = nil
	}

	return &ServiceContext{
		Config: c,
		DB:     db,
		JWT:    jwtauth.NewManager(c.Auth.AccessSecret, c.Auth.AccessExpire),
	}
}
