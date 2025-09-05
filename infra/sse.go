package infra

import (
	"bufio"
	"context"
	"log"
	"net/http"
	"signal-sse/config"
)

func ConnectToSSE(ctx context.Context, cfg *config.Config) (scanner *bufio.Scanner) {

	log.Printf("Starting Signal SSE receiver on %s...", cfg.NatsSubjectIn)

	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, "GET", cfg.SSEURLReceive, nil)
	if err != nil {
		log.Fatalf("Failed to create SSE request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Failed to connect to SSE endpoint: %v", err)
	}
	defer resp.Body.Close()

	log.Println("Connected to SSE stream.")

	scanner = bufio.NewScanner(resp.Body)

	return scanner

}
