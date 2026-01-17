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

	"github.com/xiaot623/gogo/ingress/internal/config"
	"github.com/xiaot623/gogo/ingress/internal/hub"
	internalhttp "github.com/xiaot623/gogo/ingress/internal/http"
	"github.com/xiaot623/gogo/ingress/internal/orchestrator"
	"github.com/xiaot623/gogo/ingress/internal/ws"
)

func main() {
	// Load configuration
	cfg := config.Load()

	log.Printf("Starting ingress service...")
	log.Printf("WebSocket Port: %d", cfg.WSPort)
	log.Printf("HTTP Port: %d", cfg.HTTPPort)
	log.Printf("Orchestrator URL: %s", cfg.OrchestratorURL)

	// Initialize hub
	connectionHub := hub.NewHub()
	go connectionHub.Run()

	// Initialize orchestrator client
	orchClient := orchestrator.NewClient(cfg.OrchestratorURL)

	// Initialize WebSocket server
	wsServer := ws.NewServer(cfg, connectionHub, orchClient)

	// Create WebSocket Echo server
	wsEcho := echo.New()
	wsEcho.HideBanner = true
	wsEcho.HidePort = true
	wsEcho.Use(middleware.Logger())
	wsEcho.Use(middleware.Recover())
	wsEcho.GET("/ws", wsServer.HandleWebSocket)

	// Initialize internal HTTP server
	httpServer := internalhttp.NewServer(connectionHub)

	// Start WebSocket server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.WSPort)
		if err := wsEcho.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start WebSocket server: %v", err)
		}
	}()

	// Start internal HTTP server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.HTTPPort)
		if err := httpServer.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	log.Printf("WebSocket server started on port %d", cfg.WSPort)
	log.Printf("Internal HTTP server started on port %d", cfg.HTTPPort)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down ingress...")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown both servers
	if err := wsEcho.Shutdown(shutdownCtx); err != nil {
		log.Printf("Failed to shutdown WebSocket server gracefully: %v", err)
	}
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Failed to shutdown HTTP server gracefully: %v", err)
	}

	log.Println("Ingress stopped")
}
