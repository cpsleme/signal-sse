package config

import (
	"fmt"
	"log"
	"os"
	"signal-sse/util"

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

// Configuration reads the configuration file and loads it into the struct.
func Configuration() (*Config, error) {

	configDirPath := "/etc/signal-sse"

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
  dbname: "signal_infinity_db"
  password: "securepassword"  
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

	viper.SetConfigName("config")      // File name (without the extension)
	viper.SetConfigType("yaml")        // File format (can be "json", "toml", etc.)
	viper.AddConfigPath(configDirPath) // Adds a secondary search path

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Printf("Warning: Configuration file not found. Using defaults and/or environment variables.")
		} else {
			return nil, fmt.Errorf("error reading the configuration file: %w", err)
		}
	}

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
		NatsSubjectIn:  "signal.inbound." + util.GetHostname(),
		NatsSubjectOut: "signal.outbound." + util.GetHostname(),
		NatsQueueGroup: "signal.outbound.group",
	}, nil
}
