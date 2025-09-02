package util

import (
	"os"
)

//----------------------------------------------------------------------------------------------------
// Helper Functions for Environment Variables and Hostname
//----------------------------------------------------------------------------------------------------

// getEnvAsString returns the value of an environment variable as a string,
// or a default value if the variable is not set.
func GetEnvAsString(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// valueOrDefault helps handle *string pointers safely.
func ValueOrDefault(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}
