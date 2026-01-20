#!/bin/bash
# Test script for gogo-cli connecting to ingress and invoking demo agent
#
# Prerequisites:
#   1. Start services: docker-compose up -d (or run locally)
#   2. Start demo agent: cd agent-demo && uv run agent-demo
#   3. Register demo agent with orchestrator
#
# This script will:
#   - Register demo agent (if not registered)
#   - Run the CLI client to connect and send a test message

set -e

# Configuration
ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://localhost:8080}"
INGRESS_WS_URL="${INGRESS_WS_URL:-ws://localhost:8090/ws}"
AGENT_DEMO_URL="${AGENT_DEMO_URL:-http://localhost:8000}"

echo "=== GoGo CLI Test ==="
echo "Orchestrator: $ORCHESTRATOR_URL"
echo "Ingress WS:   $INGRESS_WS_URL"
echo "Agent Demo:   $AGENT_DEMO_URL"
echo ""

# Check services are running
echo "Checking services..."

# Check orchestrator
if ! curl -s "$ORCHESTRATOR_URL/health" > /dev/null 2>&1; then
    echo "ERROR: Orchestrator is not running at $ORCHESTRATOR_URL"
    echo "Run: cd orchestrator && go run . (or docker-compose up -d)"
    exit 1
fi
echo "  [OK] Orchestrator is healthy"

# Check ingress
if ! curl -s "http://localhost:8090/health" > /dev/null 2>&1; then
    echo "ERROR: Ingress is not running at localhost:8090"
    echo "Run: cd ingress && go run . (or docker-compose up -d)"
    exit 1
fi
echo "  [OK] Ingress is healthy"

# Check agent-demo
if ! curl -s "$AGENT_DEMO_URL/health" > /dev/null 2>&1; then
    echo "ERROR: Demo agent is not running at $AGENT_DEMO_URL"
    echo "Run: cd agent-demo && uv run agent-demo"
    exit 1
fi
echo "  [OK] Demo agent is healthy"

# Register demo agent (idempotent - will update if exists)
echo ""
echo "Registering demo agent..."
REGISTER_RESPONSE=$(curl -s -X POST "$ORCHESTRATOR_URL/v1/agents/register" \
    -H "Content-Type: application/json" \
    -d '{
        "agent_id": "demo",
        "name": "Demo Agent",
        "endpoint": "'"$AGENT_DEMO_URL"'",
        "capabilities": ["streaming", "llm-mock"]
    }')
echo "  Response: $REGISTER_RESPONSE"

# List registered agents
echo ""
echo "Registered agents:"
curl -s "$ORCHESTRATOR_URL/v1/agents" | python3 -m json.tool 2>/dev/null || curl -s "$ORCHESTRATOR_URL/v1/agents"

echo ""
echo "=== Starting CLI Client ==="
echo "Connecting to $INGRESS_WS_URL with agent 'demo'"
echo "Type a message and press Enter to send."
echo "Type /quit to exit."
echo ""

# Build and run CLI
cd "$(dirname "$0")"
go build -o gogo-cli .
./gogo-cli -addr "$INGRESS_WS_URL" -agent demo
