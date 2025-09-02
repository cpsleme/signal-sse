package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"signal-sse/config"
	signal_service "signal-sse/service/signal"
	storage_service "signal-sse/service/storage"
	"syscall"

	"github.com/nats-io/nats.go"
)

//----------------------------------------------------------------------------------------------------
// Main Service Functions
//----------------------------------------------------------------------------------------------------

func main() {
	// Initialize configuration
	cfg, err := config.Configuration()
	if err != nil {
		log.Fatalf("Could not get configuraion. %v", err)
	}

	// Set up graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM) // Listen for Ctrl+C and termination signals

	go func() {
		<-signalChan // Block until a signal is received
		log.Println("Received termination signal. Shutting down services...")
		cancel() // Signal all Goroutines to stop
	}()

	// Centralized NATS connection
	log.Printf("Attempting to connect to NATS at: %s", cfg.NatsServer)
	nc, err := nats.Connect(cfg.NatsServer)
	if err != nil {
		log.Fatalf("Could not connect to NATS. %v", err)
	}
	defer nc.Close()
	log.Printf("Successfully connected to NATS at: %s", cfg.NatsServer)

	// Start Signal Services as Goroutines, passing context and config
	go signal_service.ReceiveSignalMessageService(ctx, nc, cfg)
	go signal_service.SendSignalMessageService(ctx, nc, cfg)

	// Start Storage Services as Goroutines, passing context and config
	go storage_service.StartHistoryInbound(ctx, nc, cfg)
	go storage_service.StartHistoryOutbound(ctx, nc, cfg)

	<-ctx.Done() // Wait for the context to be cancelled
	log.Println("All services stopped. Exiting application.")
}
