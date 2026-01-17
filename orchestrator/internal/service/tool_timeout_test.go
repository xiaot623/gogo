package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/xiaot623/gogo/orchestrator/internal/adapter/agentclient"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/ingress"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/llm"
	"github.com/xiaot623/gogo/orchestrator/internal/config"
	"github.com/xiaot623/gogo/orchestrator/internal/domain"
	"github.com/xiaot623/gogo/orchestrator/policy"
	"github.com/xiaot623/gogo/orchestrator/tests/helpers"
)

func TestToolCallTimeoutSweepMarksTimeout(t *testing.T) {
	ctx := context.Background()
	db := helpers.NewTestSQLiteStore(t)

	cfg := &config.Config{ToolTimeout: 50 * time.Millisecond}
	agent := agentclient.NewClient()
	ing := ingress.NewClient("")
	llmClient := llm.NewClient("", "", time.Second)
	policyEngine, err := policy.NewEngine(ctx, policy.DefaultPolicy)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	svc := New(db, agent, ing, llmClient, cfg, policyEngine)

	if err := db.CreateSession(ctx, &domain.Session{SessionID: "s1", UserID: "u1", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := db.CreateRun(ctx, &domain.Run{RunID: "r1", SessionID: "s1", RootAgentID: "a1", Status: domain.RunStatusRunning, StartedAt: time.Now()}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	tc := &domain.ToolCall{
		ToolCallID: "tc_1",
		RunID:      "r1",
		ToolName:   "browser.screenshot",
		Kind:       domain.ToolKindClient,
		Status:     domain.ToolCallStatusDispatched,
		Args:       json.RawMessage(`{}`),
		TimeoutMs:  5,
		CreatedAt:  time.Now().Add(-time.Second),
	}
	if err := db.CreateToolCall(ctx, tc); err != nil {
		t.Fatalf("CreateToolCall: %v", err)
	}

	svc.sweepToolCallTimeouts(ctx)

	got, err := db.GetToolCall(ctx, "tc_1")
	if err != nil {
		t.Fatalf("GetToolCall: %v", err)
	}
	if got == nil {
		t.Fatalf("expected tool call")
	}
	if got.Status != domain.ToolCallStatusTimeout {
		t.Fatalf("expected TIMEOUT, got %s", got.Status)
	}
	if got.CompletedAt == nil {
		t.Fatalf("expected completed_at set")
	}
	if len(got.Error) == 0 {
		t.Fatalf("expected error payload")
	}
}

func TestToolCallTimeoutSweepExpiresApproval(t *testing.T) {
	ctx := context.Background()
	db := helpers.NewTestSQLiteStore(t)

	cfg := &config.Config{ToolTimeout: 50 * time.Millisecond}
	agent := agentclient.NewClient()
	ing := ingress.NewClient("")
	llmClient := llm.NewClient("", "", time.Second)
	policyEngine, err := policy.NewEngine(ctx, policy.DefaultPolicy)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	svc := New(db, agent, ing, llmClient, cfg, policyEngine)

	if err := db.CreateSession(ctx, &domain.Session{SessionID: "s1", UserID: "u1", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := db.CreateRun(ctx, &domain.Run{RunID: "r1", SessionID: "s1", RootAgentID: "a1", Status: domain.RunStatusRunning, StartedAt: time.Now()}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	tc := &domain.ToolCall{
		ToolCallID: "tc_2",
		RunID:      "r1",
		ToolName:   "payments.transfer",
		Kind:       domain.ToolKindServer,
		Status:     domain.ToolCallStatusWaitingApproval,
		Args:       json.RawMessage(`{}`),
		ApprovalID: "ap_1",
		TimeoutMs:  5,
		CreatedAt:  time.Now().Add(-time.Second),
	}
	if err := db.CreateToolCall(ctx, tc); err != nil {
		t.Fatalf("CreateToolCall: %v", err)
	}
	if err := db.CreateApproval(ctx, &domain.Approval{
		ApprovalID: "ap_1",
		RunID:      "r1",
		ToolCallID: "tc_2",
		Status:     domain.ApprovalStatusPending,
		CreatedAt:  time.Now().Add(-time.Second),
	}); err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}

	svc.sweepToolCallTimeouts(ctx)

	ap, err := db.GetApproval(ctx, "ap_1")
	if err != nil {
		t.Fatalf("GetApproval: %v", err)
	}
	if ap == nil {
		t.Fatalf("expected approval")
	}
	if ap.Status != domain.ApprovalStatusExpired {
		t.Fatalf("expected EXPIRED, got %s", ap.Status)
	}
}
