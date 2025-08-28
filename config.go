package main

import (
	"fmt"
	"log"
	"os"

	"path/filepath"

	"github.com/spf13/viper"
)

//----------------------------------------------------------------------------------------------------
// Service Configuration
//----------------------------------------------------------------------------------------------------

// Config is the struct that will store the configuration values.
// The `mapstructure` tags are used by Viper to map the keys from
// the configuration file to the fields of your struct.
type Config struct {
	Database struct {
		Host     string `mapstructure:"host"`
		Port     int    `mapstructure:"port"`
		User     string `mapstructure:"user"`
		Password string `mapstructure:"password"`
		DBName   string `mapstructure:"dbname"`
	} `mapstructure:"database"`
	Nats struct {
		Server string `mapstructure:"server"`
		Port   int    `mapstructure:"port"`
	} `mapstructure:"nats"`
	ServerSSE struct {
		Host string `mapstructure:"host"`
		Port int    `mapstructure:"port"`
	} `mapstructure:"server-sse"`
	// Dependent variables
	SSEURLReceive  string
	SSEURLSend     string
	MySQLDSN       string
	NatsServer     string
	NatsSubjectIn  string
	NatsSubjectOut string
	NatsQueueGroup string
}

// initConfig reads the configuration file and loads it into the struct.
func initConfig() (*Config, error) {

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get user home directory: %v", err)
	}

	configDirPath := filepath.Join(homeDir, ".signal-sse")

	if err := os.MkdirAll(configDirPath, 0755); err != nil {
		log.Fatalf("Failed to create config directory '%s': %v", configDirPath, err)
	}

	configFilePath := filepath.Join(configDirPath, "config.yaml")

	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		log.Println("File 'config.yaml' not found. Creating a sample file.")
		exampleConfig := `database:
  host: "localhost"
  port: 3306
  user: "signal_user"
  password: "securepassword"
  dbname: "signal_infinity_db"
nats:
  server: "nats://localhost"
  port: 4222
server-sse:
  host: "0.0.0.0"
  port: 8080
`
		if writeErr := os.WriteFile(configFilePath, []byte(exampleConfig), 0644); writeErr != nil {
			return nil, fmt.Errorf("failed to create sample configuration file: %w", writeErr)
		}
	}

	// 1. Configures Viper to look for a configuration file.
	viper.SetConfigName("config")      // File name (without the extension)
	viper.SetConfigType("yaml")        // File format (can be "json", "toml", etc.)
	viper.AddConfigPath(".")           // Adds the current directory as a search path
	viper.AddConfigPath(configDirPath) // Adds a secondary search path

	// 2. Tries to read the configuration file.
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// File not found, but can continue with default values
			log.Printf("Warning: Configuration file not found. Using defaults and/or environment variables.")
		} else {
			return nil, fmt.Errorf("error reading the configuration file: %w", err)
		}
	}

	// 3. Optional, but recommended: Bind environment variables.
	// This allows environment variables to override values from the file.
	// For example, an environment variable 'APP_DATABASE_USER' would
	// override the 'user' field in the file.
	viper.SetEnvPrefix("APP") // Prefix for environment variables (e.g., APP_DATABASE_USER)
	viper.AutomaticEnv()      // Enables automatic reading of environment variables

	// 4. Maps the read configuration to our struct.
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("could not map the configuration to the struct: %w", err)
	}

	// Read and set dependent variables after unmarshalling
	signalSSEServer := fmt.Sprintf("%s:%d", cfg.ServerSSE.Host, cfg.ServerSSE.Port)
	sseURLReceive := fmt.Sprintf("http://%s/api/v1/events", signalSSEServer)
	sseURLSend := fmt.Sprintf("http://%s/api/v1/rpc", signalSSEServer)

	natsServer := fmt.Sprintf("%s:%d", cfg.Nats.Server, cfg.Nats.Port)

	// Construct the MySQL DSN
	mysqlDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.DBName,
	)

	// Final configuration struct with all values set
	return &Config{
		Database:       cfg.Database,
		Nats:           cfg.Nats,
		ServerSSE:      cfg.ServerSSE,
		SSEURLReceive:  sseURLReceive,
		SSEURLSend:     sseURLSend,
		MySQLDSN:       mysqlDSN,
		NatsServer:     natsServer,
		NatsSubjectIn:  "signal.inbound",
		NatsSubjectOut: "signal.outbound",
		NatsQueueGroup: "signal.outbound.group",
	}, nil
}
