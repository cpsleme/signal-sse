package main

import (
	"log"
	"os"
	"strconv"
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

// getEnvAsInt returns the value of an environment variable as an integer,
// or a default value if the variable is not set or is not a valid number.
func getEnvAsInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("Warning: Environment variable '%s' could not be converted to an integer. Using default value %d.", key, defaultValue)
		return defaultValue
	}
	return intValue
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
