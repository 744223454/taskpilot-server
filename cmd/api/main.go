package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/744223454/taskpilot-server/internal/config"
	"github.com/744223454/taskpilot-server/internal/handler"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/gin-gonic/gin"
)

var configFile = flag.String("f", "etc/taskpilot-api.yaml", "the config file")

func main() {
	flag.Parse()

	c, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	gin.SetMode(c.Mode)

	router := gin.Default()

	ctx := svc.NewServiceContext(c)
	handler.RegisterRoutes(router, ctx)

	addr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	log.Printf("starting %s at %s", c.Name, addr)

	if err := router.Run(addr); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
