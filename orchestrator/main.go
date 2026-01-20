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

	"github.com/xiaot623/gogo/orchestrator/internal/adapter/agentclient"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/ingress"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/llm"
	"github.com/xiaot623/gogo/orchestrator/internal/config"
	"github.com/xiaot623/gogo/orchestrator/internal/repository"
	"github.com/xiaot623/gogo/orchestrator/internal/service"
	transport "github.com/xiaot623/gogo/orchestrator/internal/transport/http"
	internalrpc "github.com/xiaot623/gogo/orchestrator/internal/transport/rpc"
	"github.com/xiaot623/gogo/orchestrator/policy"
)

func main() {
	// Load configuration
	cfg := config.Load()

	log.Printf("Starting orchestrator...")
	log.Printf("External HTTP Port: %d", cfg.HTTPPort)
	log.Printf("Internal RPC Port: %d", cfg.InternalPort)
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
	ingressClient := ingress.NewClient(cfg.IngressRPCAddr)

	// Initialize LLM client (uses mock if GOGO_MODE=MOCK)
	llmClient := llm.NewLLMClient(cfg.LiteLLMURL, cfg.LiteLLMAPIKey, cfg.LLMTimeout)

	// Initialize policy engine
	ctx := context.Background()
	policyEngine, err := policy.NewEngine(ctx, policy.DefaultPolicy)
	if err != nil {
		log.Fatalf("Failed to initialize policy engine: %v", err)
	}

	// Initialize service
	svc := service.New(db, agentClient, ingressClient, llmClient, cfg, policyEngine)

	// Start background monitors (best-effort)
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	go svc.RunToolCallTimeoutMonitor(bgCtx)

	// Create servers
	externalServer := transport.NewExternalServer(svc)
	rpcServer, err := internalrpc.NewServer(svc)
	if err != nil {
		log.Fatalf("Failed to initialize internal RPC server: %v", err)
	}

	// Start external server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.HTTPPort)
		if err := externalServer.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start external server: %v", err)
		}
	}()

	// Start internal RPC server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.InternalPort)
		if err := rpcServer.Start(addr); err != nil {
			log.Fatalf("Failed to start internal RPC server: %v", err)
		}
	}()

	log.Printf("External API started on port %d", cfg.HTTPPort)
	log.Printf("Internal RPC started on port %d", cfg.InternalPort)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down orchestrator...")
	bgCancel()

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown both servers
	if err := externalServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Failed to shutdown external server gracefully: %v", err)
	}
	if err := rpcServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Failed to shutdown internal RPC server gracefully: %v", err)
	}

	log.Println("Orchestrator stopped")
}
