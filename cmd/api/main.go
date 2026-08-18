package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/744223454/taskpilot-server/internal/config"
	"github.com/744223454/taskpilot-server/internal/handler"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/pkg/ai"
	"github.com/gin-gonic/gin"
)

var configFile = flag.String("f", "etc/taskpilot-api.yaml", "the config file")

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 0
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("run API: %v", err)
	}
}

func run() (runErr error) {
	flag.Parse()

	c, err := config.Load(*configFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	gin.SetMode(c.Mode)

	router := gin.Default()
	if err := router.SetTrustedProxies(c.HTTP.TrustedProxies); err != nil {
		return fmt.Errorf("configure trusted proxies: %w", err)
	}

	serverContext := svc.NewServiceContext(c)
	if c.AI.APIKey != "" {
		chatClient, chatErr := ai.NewResponsesChatClient(
			c.AI.BaseURL,
			c.AI.APIKey,
			c.AI.Model,
			time.Duration(c.AI.ChatRequestTimeout)*time.Second,
			c.AI.ChatMaxOutputTokens,
		)
		if chatErr != nil {
			serverContext.Logger.Warn("AI chat initialization failed", "error", chatErr)
		} else {
			serverContext.Chat = chatClient
		}
	} else {
		serverContext.Logger.Warn("AI chat initialization skipped", "error", "config AI.APIKey is empty")
	}
	defer func() {
		if err := serverContext.Close(); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}()
	handler.RegisterRoutes(router, serverContext)

	addr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	log.Printf("starting %s at %s", c.Name, addr)

	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	shutdownSignal, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("listen and serve: %w", err)
	case <-shutdownSignal.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			closeErr := server.Close()
			<-serverErrors
			return errors.Join(fmt.Errorf("shutdown server: %w", err), closeErr)
		}
		if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("stop server: %w", err)
		}
		return nil
	}
}
