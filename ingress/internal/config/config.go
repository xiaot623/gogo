// Package config provides configuration for the ingress service.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds the ingress configuration.
type Config struct {
	// Server settings
	WSPort   int // External WebSocket port
	RPCPort  int // Internal RPC port for ingress events

	// Orchestrator settings (RPC address)
	OrchestratorRPCAddr string

	// Auth settings
	APIKey string // Static API key for hello.api_key validation

	// WebSocket settings
	PingInterval   time.Duration
	WriteTimeout   time.Duration
	ReadTimeout    time.Duration
	MaxMessageSize int64

	// Logging
	LogLevel string
}

// Load loads configuration from environment variables.
func Load() *Config {
	return &Config{
		WSPort:              getEnvInt("WS_PORT", 8090),
		RPCPort:             getEnvIntWithFallback("RPC_PORT", "HTTP_PORT", 8091),
		OrchestratorRPCAddr: getEnvWithFallback("ORCHESTRATOR_RPC_ADDR", "ORCHESTRATOR_URL", "orchestrator:8081"),
		APIKey:              getEnv("API_KEY", ""),
		PingInterval:        time.Duration(getEnvInt("WS_PING_INTERVAL_MS", 30000)) * time.Millisecond,
		WriteTimeout:        time.Duration(getEnvInt("WS_WRITE_TIMEOUT_MS", 10000)) * time.Millisecond,
		ReadTimeout:         time.Duration(getEnvInt("WS_READ_TIMEOUT_MS", 60000)) * time.Millisecond,
		MaxMessageSize:      int64(getEnvInt("WS_MAX_MESSAGE_SIZE", 65536)),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
	}
}

func getEnvWithFallback(primary, fallback, defaultVal string) string {
	if val := os.Getenv(primary); val != "" {
		return val
	}
	if val := os.Getenv(fallback); val != "" {
		return val
	}
	return defaultVal
}

func getEnvIntWithFallback(primary, fallback string, defaultVal int) int {
	if val := os.Getenv(primary); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
		return defaultVal
	}
	if val := os.Getenv(fallback); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
		return defaultVal
	}
	return defaultVal
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}
