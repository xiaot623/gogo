# Demo Agent

A basic agent implementation for testing the multi-agent platform (M0 milestone).

## Features

- **SSE Streaming**: Returns responses as Server-Sent Events
- **Mock LLM**: Simulates LLM responses with configurable streaming delay
- **Health Check**: Standard health endpoint for service discovery

## Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/invoke` | Agent invocation (SSE response) |
| GET | `/` | Service info |

## Quick Start

### Install Dependencies

```bash
cd agent-demo
uv sync
```

### Run the Agent

```bash
# Using uv
uv run agent-demo

# Or with uvicorn directly
uv run uvicorn agent_demo.app:app --host 0.0.0.0 --port 8000 --reload
```

### Test Locally

```bash
# Health check
curl http://localhost:8000/health

# Invoke (watch SSE stream)
curl -N -X POST http://localhost:8000/invoke \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "demo",
    "session_id": "sess_001",
    "run_id": "run_001",
    "input_message": {"role": "user", "content": "hello"}
  }'
```

## Integration with Orchestrator

1. Start the orchestrator:
```bash
cd orchestrator && go build && ./orchestrator
```

2. Start the demo agent:
```bash
cd agent-demo && uv run agent-demo
```

3. Register the demo agent:
```bash
curl -X POST http://localhost:8080/v1/agents/register \
  -H "Content-Type: application/json" \
  -d '{"agent_id":"demo","name":"Demo Agent","endpoint":"http://localhost:8000"}'
```

4. Invoke through ingress (WebSocket) or the internal RPC `Orchestrator.Invoke` method.

## SSE Event Format

### Delta Event (streaming chunk)
```
event: delta
data: {"text": "Hello", "run_id": "run_001"}
```

### Done Event (completion)
```
event: done
data: {"final_message": "Hello! I'm the demo agent.", "usage": {"tokens": 10}}
```

## Mock Responses

The agent recognizes these keywords and responds accordingly:

| Keyword | Response |
|---------|----------|
| "hello" | Greeting message |
| "weather" | Weather information |
| "help" | Help message |
| (other) | Echo with explanation |

## Project Structure

```
agent-demo/
├── pyproject.toml       # Project configuration
├── README.md
└── src/
    └── agent_demo/
        ├── __init__.py  # Package init
        ├── app.py       # FastAPI application
        ├── main.py      # Entry point
        ├── models.py    # Pydantic models
        └── llm.py       # Mock LLM logic
```
