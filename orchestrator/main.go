package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/xiaot623/gogo/orchestrator/agentclient"
	"github.com/xiaot623/gogo/orchestrator/api"
	"github.com/xiaot623/gogo/orchestrator/config"
	"github.com/xiaot623/gogo/orchestrator/llmproxy"
	"github.com/xiaot623/gogo/orchestrator/policy"
	"github.com/xiaot623/gogo/orchestrator/store"
)

func main() {
	// Load configuration
	cfg := config.Load()

	log.Printf("Starting orchestrator...")
	log.Printf("HTTP Port: %d", cfg.HTTPPort)
	log.Printf("Database: %s", cfg.DatabaseURL)
	log.Printf("LiteLLM URL: %s", cfg.LiteLLMURL)

	// Initialize store
	db, err := store.NewSQLiteStore(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}
	defer db.Close()

	// Initialize agent client
	agentClient := agentclient.NewClient()

	// Initialize policy engine
	ctx := context.Background()
	policyEngine, err := policy.NewEngine(ctx, policy.DefaultPolicy)
	if err != nil {
		log.Fatalf("Failed to initialize policy engine: %v", err)
	}

	// Initialize handler
	handler := api.NewHandler(db, agentClient, cfg, policyEngine)

	// Initialize LLM proxy handler
	llmHandler := llmproxy.NewHandler(cfg, db)

	// Create Echo server
	e := echo.New()
	e.HideBanner = true

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Register routes
	handler.RegisterRoutes(e)
	llmHandler.RegisterRoutes(e)

	// Start server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.HTTPPort)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	log.Printf("Orchestrator started on port %d", cfg.HTTPPort)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down orchestrator...")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Printf("Failed to shutdown server gracefully: %v", err)
	}

	log.Println("Orchestrator stopped")
}
