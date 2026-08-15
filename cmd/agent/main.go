package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"opennhrp-manager/internal/agent"
	"opennhrp-manager/internal/config"
)

func main() {
	cfg := config.LoadConfig()
	serverAddrFlag := flag.String("server", "ws://127.0.0.1:8080/api/agent/ws", "Manager Server WebSocket address")
	nodeIDFlag := flag.String("node-id", "", "Unique Node ID (e.g. hub-backup1)")
	nodeTypeFlag := flag.String("node-type", cfg.NodeType, "Node type: hub or spoke")
	authTokenFlag := flag.String("token", cfg.AuthToken, "Authentication token")
	flag.Parse()
	if *nodeTypeFlag != "hub" && *nodeTypeFlag != "spoke" {
		log.Fatalf("invalid --node-type %q: must be hub or spoke", *nodeTypeFlag)
	}

	nodeID := *nodeIDFlag
	if nodeID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			hostname = "hub-node"
		}
		nodeID = hostname
	}

	log.Println("=========================================================")
	log.Println("             OpenNHRP Lightweight Agent v1.0            ")
	log.Println("=========================================================")
	log.Printf("Node ID      : %s", nodeID)
	log.Printf("Node Type    : %s", *nodeTypeFlag)
	log.Printf("Server Target: %s", *serverAddrFlag)

	client := agent.NewAgentClient(nodeID, *nodeTypeFlag, *serverAddrFlag, *authTokenFlag, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go client.Start(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down OpenNHRP Agent...")
}
