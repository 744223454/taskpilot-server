package main

import (
	"context"
	"flag"
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
	flag.Parse()

	workerConfig, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	parser, err := ai.NewResponsesParser(
		workerConfig.AI.BaseURL,
		workerConfig.AI.APIKey,
		workerConfig.AI.Model,
		time.Duration(workerConfig.AI.RequestTimeout)*time.Second,
		workerConfig.AI.MaxOutputTokens,
	)
	if err != nil {
		log.Fatalf("initialize AI parser: %v", err)
	}

	serviceContext := svc.NewServiceContext(workerConfig)
	serviceContext.Parser = parser
	worker, err := parseworker.New(serviceContext)
	if err != nil {
		log.Fatalf("initialize parse worker: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := worker.Run(ctx); err != nil {
		log.Fatalf("run parse worker: %v", err)
	}
}
