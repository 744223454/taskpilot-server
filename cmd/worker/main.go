package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/744223454/taskpilot-server/internal/config"
	"github.com/744223454/taskpilot-server/internal/svc"
	parseworker "github.com/744223454/taskpilot-server/internal/worker/parsejob"
	"github.com/744223454/taskpilot-server/pkg/ai"
)

var configFile = flag.String("f", "etc/taskpilot-api.yaml", "the config file")

func main() {
	if err := run(); err != nil {
		log.Fatalf("run parse worker: %v", err)
	}
}

func run() (runErr error) {
	flag.Parse()

	workerConfig, err := config.Load(*configFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	parser, err := ai.NewResponsesParser(
		workerConfig.AI.BaseURL,
		workerConfig.AI.APIKey,
		workerConfig.AI.Model,
		time.Duration(workerConfig.AI.RequestTimeout)*time.Second,
		workerConfig.AI.MaxOutputTokens,
	)
	if err != nil {
		return fmt.Errorf("initialize AI parser: %w", err)
	}

	serviceContext := svc.NewServiceContext(workerConfig)
	defer func() {
		if err := serviceContext.Close(); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}()
	serviceContext.Parser = parser
	worker, err := parseworker.New(serviceContext)
	if err != nil {
		return fmt.Errorf("initialize parse worker: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := worker.Run(ctx); err != nil {
		return fmt.Errorf("run parse worker: %w", err)
	}
	return nil
}
