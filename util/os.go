package util

import (
	"log"
	"os"
)

// getHostname returns the local hostname or a default string if an error occurs.
func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("Error getting hostname: %v", err)
		return "unknown-host"
	}
	return hostname
}
