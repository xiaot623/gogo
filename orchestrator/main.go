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
	
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/agentclient"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/ingress"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/llm"
	"github.com/xiaot623/gogo/orchestrator/internal/config"
	"github.com/xiaot623/gogo/orchestrator/internal/repository"
	"github.com/xiaot623/gogo/orchestrator/internal/service"
	handler "github.com/xiaot623/gogo/orchestrator/internal/transport/http"
	"github.com/xiaot623/gogo/orchestrator/internal/transport/http/internalapi"
	"github.com/xiaot623/gogo/orchestrator/internal/transport/http/llmproxy"
	"github.com/xiaot623/gogo/orchestrator/policy"
)

func main() {
	// Load configuration
	cfg := config.Load()

	log.Printf("Starting orchestrator...")
	log.Printf("External HTTP Port: %d", cfg.HTTPPort)
	log.Printf("Internal HTTP Port: %d", cfg.InternalPort)
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

	// Initialize ingress client
	ingressClient := ingress.NewClient(cfg.IngressURL)

	// Initialize LLM client
	llmClient := llm.NewClient(cfg.LiteLLMURL, cfg.LiteLLMAPIKey, cfg.LLMTimeout)

	// Initialize policy engine
	ctx := context.Background()
	policyEngine, err := policy.NewEngine(ctx, policy.DefaultPolicy)
	if err != nil {
		log.Fatalf("Failed to initialize policy engine: %v", err)
	}

	// Initialize service
	svc := service.New(db, agentClient, ingressClient, llmClient, cfg, policyEngine)

	// Initialize handlers
	h := handler.NewHandler(svc)
	internalH := internalapi.NewHandler(svc)
	llmH := llmproxy.NewHandler(svc)

	// Create external Echo server
	externalServer := echo.New()
	externalServer.HideBanner = true

	// Middleware
	externalServer.Use(middleware.Logger())
	externalServer.Use(middleware.Recover())
	externalServer.Use(middleware.CORS())

	// Register external routes (for agents to call platform)
	h.RegisterRoutes(externalServer)
	llmH.RegisterRoutes(externalServer)

	// Create internal Echo server (for ingress only)
	internalServer := echo.New()
	internalServer.HideBanner = true

	// Middleware
	internalServer.Use(middleware.Logger())
	internalServer.Use(middleware.Recover())

	// Register internal routes
	internalH.RegisterRoutes(internalServer)

	// Start external server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.HTTPPort)
		if err := externalServer.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start external server: %v", err)
		}
	}()

	// Start internal server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.InternalPort)
		if err := internalServer.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start internal server: %v", err)
		}
	}()

	log.Printf("External API started on port %d", cfg.HTTPPort)
	log.Printf("Internal API started on port %d", cfg.InternalPort)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down orchestrator...")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown both servers
	if err := externalServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Failed to shutdown external server gracefully: %v", err)
	}
	if err := internalServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Failed to shutdown internal server gracefully: %v", err)
	}

	log.Println("Orchestrator stopped")
}