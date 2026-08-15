package main

import (
	"context"
	"embed"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"opennhrp-manager/internal/api"
	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/service"
)

//go:embed all:frontend/dist
var webAssets embed.FS

func main() {
	portFlag := flag.String("port", "", "Server HTTP port (overrides PORT env)")
	dbPathFlag := flag.String("db", "", "SQLite database path")
	flag.Parse()

	cfg := config.LoadConfig()
	if *portFlag != "" {
		cfg.ServerPort = *portFlag
	}
	if *dbPathFlag != "" {
		cfg.DatabasePath = *dbPathFlag
	}

	log.Println("=========================================================")
	log.Println("       OpenNHRP Web Manager & Witness Center v1.0        ")
	log.Println("=========================================================")
	log.Printf("Architecture      : Agent-Driven Control Plane (No local sockets)")
	log.Printf("Database Path     : %s", cfg.DatabasePath)
	log.Printf("Agent WS Endpoint : /api/agent/ws")
	log.Printf("Witness Engine    : %v (Interval: %ds)", cfg.WitnessEnabled, cfg.WitnessInterval)

	database, err := db.InitDB(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logHub := service.NewLogHub()
	nodeMgr := service.NewNodeManager(cfg, database, logHub)
	witnessSvc := service.NewWitnessService(cfg, database, nodeMgr)
	provService := service.NewProvisioningService(nodeMgr)

	// Start Background Engines
	go nodeMgr.Start(ctx)
	go witnessSvc.Start(ctx)

	router := api.SetupRouter(cfg, database, nodeMgr, witnessSvc, provService, logHub)
	api.ServeSPA(router, webAssets)

	bindAddr := net.JoinHostPort(cfg.BindHost, cfg.ServerPort)
	log.Printf("Web Console & API listening at http://%s", bindAddr)

	go func() {
		if err := router.Run(bindAddr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down OpenNHRP Manager gracefully...")
	cancel()
	time.Sleep(500 * time.Millisecond)
}
