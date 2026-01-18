package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ExecutorFunc defines a server-side tool executor.
type ExecutorFunc func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

// Registry stores tool executors keyed by tool name.
type Registry struct {
	mu        sync.RWMutex
	executors map[string]ExecutorFunc
}

// DefaultRegistry is the shared registry used by the orchestrator.
var DefaultRegistry = NewRegistry()

// NewRegistry creates an empty tool executor registry.
func NewRegistry() *Registry {
	return &Registry{
		executors: make(map[string]ExecutorFunc),
	}
}

// Register adds a new executor for a tool name.
func (r *Registry) Register(toolName string, exec ExecutorFunc) error {
	if toolName == "" {
		return fmt.Errorf("tool name is required")
	}
	if exec == nil {
		return fmt.Errorf("executor is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.executors[toolName]; exists {
		return fmt.Errorf("executor already registered for %s", toolName)
	}
	r.executors[toolName] = exec
	return nil
}

// Execute runs the executor for the tool name.
func (r *Registry) Execute(ctx context.Context, toolName string, args json.RawMessage) (json.RawMessage, error) {
	if toolName == "" {
		return nil, fmt.Errorf("tool name is required")
	}
	r.mu.RLock()
	exec := r.executors[toolName]
	r.mu.RUnlock()
	if exec == nil {
		return nil, fmt.Errorf("no executor registered for %s", toolName)
	}
	return exec(ctx, args)
}

// Register adds an executor to the default registry.
func Register(toolName string, exec ExecutorFunc) error {
	return DefaultRegistry.Register(toolName, exec)
}

// MustRegister adds an executor to the default registry or panics.
func MustRegister(toolName string, exec ExecutorFunc) {
	if err := Register(toolName, exec); err != nil {
		panic(err)
	}
}
