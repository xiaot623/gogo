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
	"github.com/xiaot623/gogo/ingress/internal/orchestrator"
	internalrpc "github.com/xiaot623/gogo/ingress/internal/transport/rpc"
	"github.com/xiaot623/gogo/ingress/internal/ws"
)

func main() {
	// Load configuration
	cfg := config.Load()

	log.Printf("Starting ingress service...")
	log.Printf("WebSocket Port: %d", cfg.WSPort)
	log.Printf("RPC Port: %d", cfg.RPCPort)
	log.Printf("Orchestrator RPC Address: %s", cfg.OrchestratorRPCAddr)

	// Initialize hub
	connectionHub := hub.NewHub()
	go connectionHub.Run()

	// Initialize orchestrator client
	orchClient := orchestrator.NewClient(cfg.OrchestratorRPCAddr)

	// Initialize WebSocket server
	wsServer := ws.NewServer(cfg, connectionHub, orchClient)

	// Create WebSocket Echo server
	wsEcho := echo.New()
	wsEcho.HideBanner = true
	wsEcho.HidePort = true
	wsEcho.Use(middleware.Logger())
	wsEcho.Use(middleware.Recover())
	wsEcho.GET("/ws", wsServer.HandleWebSocket)
	wsEcho.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status":      "healthy",
			"connections": connectionHub.GetConnectionCount(),
			"sessions":    connectionHub.GetSessionCount(),
		})
	})

	// Initialize internal RPC server
	rpcServer, err := internalrpc.NewServer(connectionHub)
	if err != nil {
		log.Fatalf("Failed to initialize RPC server: %v", err)
	}

	// Start WebSocket server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.WSPort)
		if err := wsEcho.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start WebSocket server: %v", err)
		}
	}()

	// Start internal RPC server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.RPCPort)
		if err := rpcServer.Start(addr); err != nil {
			log.Fatalf("Failed to start RPC server: %v", err)
		}
	}()

	log.Printf("WebSocket server started on port %d", cfg.WSPort)
	log.Printf("Internal RPC server started on port %d", cfg.RPCPort)

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
	if err := rpcServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Failed to shutdown RPC server gracefully: %v", err)
	}

	log.Println("Ingress stopped")
}
