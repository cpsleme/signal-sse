package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"
)

//----------------------------------------------------------------------------------------------------
// Main Service Functions
//----------------------------------------------------------------------------------------------------

func main() {
	// Initialize configuration
	cfg := Configuration()

	// Set up graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM) // Listen for Ctrl+C and termination signals

	go func() {
		<-signalChan // Block until a signal is received
		log.Println("main: Received termination signal. Shutting down services...")
		cancel() // Signal all Goroutines to stop
	}()

	// Centralized NATS connection
	log.Printf("main: Attempting to connect to NATS at: %s", cfg.NatsServer)
	nc, err := nats.Connect(cfg.NatsServer)
	if err != nil {
		log.Fatalf("Fatal Error: main: Could not connect to NATS. %v", err)
	}
	defer nc.Close()
	log.Printf("main: Successfully connected to NATS at: %s", cfg.NatsServer)

	// Start services as Goroutines, passing context and config
	go receiveSignalMessageService(ctx, nc, cfg)
	go sendSignalMessageService(ctx, nc, cfg)
	go startStorage(ctx, nc, cfg)

	<-ctx.Done() // Wait for the context to be cancelled
	log.Println("main: All services stopped. Exiting application.")
}
