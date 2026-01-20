package service

import (
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/agentclient"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/ingress"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/llm"
	"github.com/xiaot623/gogo/orchestrator/internal/config"
	"github.com/xiaot623/gogo/orchestrator/internal/repository"
	"github.com/xiaot623/gogo/orchestrator/internal/tools"
	"github.com/xiaot623/gogo/orchestrator/policy"
)

type Service struct {
	store         store.Store
	agentClient   *agentclient.Client
	ingressClient *ingress.Client
	llmClient     llm.LLMClient
	config        *config.Config
	policyEngine  *policy.Engine
	toolRegistry  *tools.Registry
}

type Option func(*Service)

// WithToolRegistry overrides the default tool executor registry.
func WithToolRegistry(registry *tools.Registry) Option {
	return func(s *Service) {
		if registry != nil {
			s.toolRegistry = registry
		}
	}
}

func New(store store.Store, agentClient *agentclient.Client, ingressClient *ingress.Client, llmClient llm.LLMClient, cfg *config.Config, policyEngine *policy.Engine, opts ...Option) *Service {
	svc := &Service{
		store:         store,
		agentClient:   agentClient,
		ingressClient: ingressClient,
		llmClient:     llmClient,
		config:        cfg,
		policyEngine:  policyEngine,
		toolRegistry:  tools.DefaultRegistry,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}
