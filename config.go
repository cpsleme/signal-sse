package main

import "fmt"

//----------------------------------------------------------------------------------------------------
// Service Configuration
//----------------------------------------------------------------------------------------------------

// Config holds the service's configuration.
type Config struct {
	SignalSSEServer string
	NatsServer      string
	NatsSubjectIn   string
	NatsSubjectOut  string
	QueueGroup      string
	SSEURLReceive   string
	SSEURLSend      string
	StreamOut       string
	MySQLDSN        string
}

// Configuration creates and validates the application configuration.
func Configuration() *Config {
	// Use the generic function with proper type-specific calls
	signalSSEServer := getEnvAsString("SIGNAL_SSE_SERVER", "localhost:8080")
	natsServer := getEnvAsString("NATS_SERVER", "nats://localhost:4222")
	streamOutName := getEnvAsString("NATS_STREAM_OUT", "SIGNAL_OUTBOUND_STREAM")
	mysqlDSN := getEnvAsString("MYSQL_DSN", "signal_user:password@tcp(localhost:3306)/signal_infinity_db?parseTime=true")

	// Dependent variables
	sseURLReceive := fmt.Sprintf("http://%s/api/v1/events", signalSSEServer)
	sseURLSend := fmt.Sprintf("http://%s/api/v1/rpc", signalSSEServer)

	return &Config{
		SignalSSEServer: signalSSEServer,
		NatsServer:      natsServer,
		NatsSubjectIn:   "signal.inbound",        // NATS topic for incoming Signal messages
		NatsSubjectOut:  "signal.outbound",       // NATS topic for outgoing Signal messages
		QueueGroup:      "signal.outbound.group", // NATS Queue Group for outbound messages
		StreamOut:       streamOutName,           // Dedicated JetStream stream for outbound
		SSEURLReceive:   sseURLReceive,
		SSEURLSend:      sseURLSend,
		MySQLDSN:        mysqlDSN,
	}
}
