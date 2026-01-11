// Package config provides configuration for the orchestrator.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds the orchestrator configuration.
type Config struct {
	// Server settings
	HTTPPort     int
	InternalPort int

	// Database
	DatabaseURL string

	// Ingress settings
	IngressURL string

	// Timeouts
	AgentTimeout    time.Duration
	ToolTimeout     time.Duration
	ApprovalTimeout time.Duration

	// Logging
	LogLevel string
}

// Load loads configuration from environment variables.
func Load() *Config {
	cfg := &Config{
		HTTPPort:        getEnvInt("HTTP_PORT", 8080),
		InternalPort:    getEnvInt("INTERNAL_PORT", 8081),
		DatabaseURL:     getEnv("DATABASE_URL", "file:orchestrator.db?cache=shared&mode=rwc"),
		IngressURL:      getEnv("INGRESS_URL", "http://localhost:8090"),
		AgentTimeout:    time.Duration(getEnvInt("AGENT_TIMEOUT_MS", 300000)) * time.Millisecond,
		ToolTimeout:     time.Duration(getEnvInt("TOOL_TIMEOUT_MS", 60000)) * time.Millisecond,
		ApprovalTimeout: time.Duration(getEnvInt("APPROVAL_TIMEOUT_MS", 600000)) * time.Millisecond,
		LogLevel:        getEnv("LOG_LEVEL", "info"),
	}
	return cfg
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
