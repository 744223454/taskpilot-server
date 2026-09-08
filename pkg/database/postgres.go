package database

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	connectRetryInterval = 2 * time.Second
	connectRetryTimeout  = 2 * time.Minute
)

// NewPostgres opens a shared Gorm connection for the application.
// It retries until the timeout elapses so the process survives slow
// dependency startup (e.g. Postgres crash recovery after a host reboot)
// instead of starting permanently degraded.
func NewPostgres(dsn string) (*gorm.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("postgres data source is required")
	}

	deadline := time.Now().Add(connectRetryTimeout)
	for {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
			TranslateError: true,
		})
		if err == nil {
			return db, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
		slog.Warn("postgres not ready, retrying", "error", err, "retry_in", connectRetryInterval.String())
		time.Sleep(connectRetryInterval)
	}
}
