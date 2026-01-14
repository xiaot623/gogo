package policy

import (
	"context"
	"fmt"

	"github.com/open-policy-agent/opa/rego"
)

// Engine is the OPA policy engine.
type Engine struct {
	query rego.PreparedEvalQuery
}

// NewEngine creates a new policy engine with the given policy content.
func NewEngine(ctx context.Context, policyContent string) (*Engine, error) {
	// Simple module for now
	r := rego.New(
		rego.Query("data.tool_policy.decision"),
		rego.Module("tool_policy.rego", policyContent),
	)

	query, err := r.PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare rego: %w", err)
	}

	return &Engine{query: query}, nil
}

// Evaluate checks the tool policy.
// Input should be a map with keys: tool_name, args, user_id, etc.
// Returns: decision (allow, require_approval, block), reason (optional), error
func (e *Engine) Evaluate(ctx context.Context, input interface{}) (string, string, error) {
	results, err := e.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return "", "", fmt.Errorf("failed to evaluate policy: %w", err)
	}

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		// Default to allow if no rules match? Or should the policy define a default?
		// We assume the policy defines a default.
		return "allow", "default", nil
	}

	// Expecting result to be a string or an object {decision: "...", reason: "..."}
	// For simplicity, let's assume the rule returns a string first, or we can make it return an object.
	// In the plan, we used `decision = "..."`.

	val := results[0].Expressions[0].Value
	if s, ok := val.(string); ok {
		return s, "", nil
	}

	// Handle object return if needed in future
	return "allow", "unexpected return type", nil
}

// DefaultPolicy is the default policy content.
const DefaultPolicy = `
package tool_policy

default decision = "allow"

# Example: Block dangerous tools
decision = "block" {
	input.tool_name == "dangerous.command"
}

# Example: Require approval for high value transfer
decision = "require_approval" {
	input.tool_name == "payments.transfer"
	input.args.amount > 100
}
`
