package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

func (s *Service) RegisterAgent(ctx context.Context, agentID, name, endpoint string, capabilities []string) (*domain.Agent, error) {
	caps, _ := json.Marshal(capabilities)
	now := time.Now()
	agent := &domain.Agent{
		AgentID:      agentID,
		Name:         name,
		Endpoint:     endpoint,
		Capabilities: caps,
		Status:       "healthy",
		CreatedAt:    now,
	}

	if err := s.store.RegisterAgent(ctx, agent); err != nil {
		return nil, fmt.Errorf("failed to register agent: %w", err)
	}

	return agent, nil
}

func (s *Service) ListAgents(ctx context.Context) ([]domain.Agent, error) {
	agents, err := s.store.ListAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}
	return agents, nil
}

func (s *Service) GetAgent(ctx context.Context, agentID string) (*domain.Agent, error) {
	agent, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	return agent, nil
}
