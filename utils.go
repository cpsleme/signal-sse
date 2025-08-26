package main

import (
	"log"
	"os"
)

//----------------------------------------------------------------------------------------------------
// Helper Functions for Environment Variables and Hostname
//----------------------------------------------------------------------------------------------------

// getEnvAsString returns the value of an environment variable as a string,
// or a default value if the variable is not set.
func getEnvAsString(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// getHostname returns the local hostname or a default string if an error occurs.
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("Error getting hostname: %v", err)
		return "unknown-host"
	}
	return hostname
}

// valueOrDefault helps handle *string pointers safely.
func valueOrDefault(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}
