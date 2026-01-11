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

	"github.com/gogo/orchestrator/agentclient"
	"github.com/gogo/orchestrator/api"
	"github.com/gogo/orchestrator/config"
	"github.com/gogo/orchestrator/store"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Load configuration
	cfg := config.Load()

	log.Printf("Starting orchestrator...")
	log.Printf("HTTP Port: %d", cfg.HTTPPort)
	log.Printf("Database: %s", cfg.DatabaseURL)

	// Initialize store
	db, err := store.NewSQLiteStore(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}
	defer db.Close()

	// Initialize agent client
	agentClient := agentclient.NewClient()

	// Initialize handler
	handler := api.NewHandler(db, agentClient, cfg)

	// Create Echo server
	e := echo.New()
	e.HideBanner = true

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Register routes
	handler.RegisterRoutes(e)

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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(ctx); err != nil {
		log.Printf("Failed to shutdown server gracefully: %v", err)
	}

	log.Println("Orchestrator stopped")
}
