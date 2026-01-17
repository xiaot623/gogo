package service

import (
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/agentclient"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/ingress"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/llm"
	"github.com/xiaot623/gogo/orchestrator/internal/config"
	"github.com/xiaot623/gogo/orchestrator/internal/repository"
	"github.com/xiaot623/gogo/orchestrator/policy"
)

type Service struct {
	store         store.Store
	agentClient   *agentclient.Client
	ingressClient *ingress.Client
	llmClient     *llm.Client
	config        *config.Config
	policyEngine  *policy.Engine
}

func New(store store.Store, agentClient *agentclient.Client, ingressClient *ingress.Client, llmClient *llm.Client, cfg *config.Config, policyEngine *policy.Engine) *Service {
	return &Service{
		store:         store,
		agentClient:   agentClient,
		ingressClient: ingressClient,
		llmClient:     llmClient,
		config:        cfg,
		policyEngine:  policyEngine,
	}
}
