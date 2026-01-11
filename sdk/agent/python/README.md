# Agent SDK for Multi-Agent Platform

Python SDK for building agents on the multi-agent platform.

## Installation

```bash
# Using uv
uv add agent-sdk

# Using pip
pip install agent-sdk
```

## Quick Start

### Method 1: Subclass Agent

```python
import asyncio
from collections.abc import AsyncIterator

from agent_sdk import Agent, InvokeContext, SSEResponse, Usage


class MyAgent(Agent):
    """A simple agent that echoes user input."""

    async def invoke(self, ctx: InvokeContext) -> AsyncIterator[str]:
        sse = SSEResponse(run_id=ctx.run_id)

        # Process the input
        response = f"You said: {ctx.input_message.content}"

        # Stream the response
        for char in response:
            yield sse.delta(char)
            await asyncio.sleep(0.02)

        # Send completion event
        yield sse.done(final_message=response)


# Create and run the agent
agent = MyAgent(
    agent_id="my-agent",
    name="My Agent",
    version="1.0.0",
    capabilities=["echo"],
)

if __name__ == "__main__":
    agent.run(port=8000)
```

### Method 2: Use Decorator

```python
from collections.abc import AsyncIterator

from agent_sdk import create_agent, InvokeContext, SSEResponse


@create_agent("echo", "Echo Agent", version="1.0.0")
async def echo_agent(ctx: InvokeContext) -> AsyncIterator[str]:
    sse = SSEResponse(run_id=ctx.run_id)
    yield sse.delta(ctx.input_message.content)
    yield sse.done(final_message=ctx.input_message.content)


if __name__ == "__main__":
    echo_agent.run(port=8000)
```

## Using Platform Services

Agents can interact with platform services through the `PlatformClient`:

```python
from agent_sdk import Agent, InvokeContext, PlatformClient, SSEResponse


class SmartAgent(Agent):
    async def invoke(self, ctx: InvokeContext) -> AsyncIterator[str]:
        sse = SSEResponse(run_id=ctx.run_id)

        # Create platform client
        async with PlatformClient(
            base_url=ctx.platform_base_url or "http://orchestrator:8080",
            run_id=ctx.run_id,
        ) as client:
            # Call LLM through platform proxy
            response = await client.llm.chat_completions(
                model="gpt-4",
                messages=[
                    {"role": "system", "content": "You are a helpful assistant."},
                    {"role": "user", "content": ctx.input_message.content},
                ],
            )
            
            result = response["choices"][0]["message"]["content"]
            yield sse.delta(result)
            yield sse.done(final_message=result)
```

### Streaming LLM Responses

```python
async with PlatformClient(base_url, run_id) as client:
    full_response = ""
    async for chunk in client.llm.chat_completions_stream(
        model="gpt-4",
        messages=[{"role": "user", "content": "Hello"}],
    ):
        text = chunk["choices"][0]["delta"].get("content", "")
        full_response += text
        yield sse.delta(text)
    
    yield sse.done(final_message=full_response)
```

### Calling Tools

```python
async with PlatformClient(base_url, run_id) as client:
    # Invoke and wait for completion
    result = await client.tools.invoke_and_wait(
        tool_name="weather.query",
        args={"city": "Beijing"},
    )
    
    if result.succeeded:
        weather_data = result.result
    else:
        error = result.error
```

### Agent-to-Agent Calls

```python
async with PlatformClient(base_url, run_id) as client:
    async for event in client.agents.invoke(
        agent_id="weather_agent",
        message={"role": "user", "content": "What's the weather?"},
    ):
        if event["type"] == "delta":
            yield sse.delta(event["data"]["text"])
        elif event["type"] == "done":
            break
```

## API Reference

### Agent Base Class

```python
class Agent(ABC):
    def __init__(
        self,
        agent_id: str,          # Unique identifier
        name: str,              # Human-readable name
        version: str = "0.1.0", # Version string
        capabilities: list[str] = None,  # Capability list
    )

    @abstractmethod
    async def invoke(self, ctx: InvokeContext) -> AsyncIterator[str]:
        """Handle an invocation request. Yield SSE-formatted events."""
        pass

    async def on_startup(self) -> None:
        """Called on server startup. Override for initialization."""
        pass

    async def on_shutdown(self) -> None:
        """Called on server shutdown. Override for cleanup."""
        pass

    def run(
        self,
        host: str = "0.0.0.0",
        port: int = 8000,
        log_level: str = "info",
    ) -> None:
        """Run the agent server."""
        pass
```

### InvokeContext

```python
class InvokeContext:
    agent_id: str              # Agent being invoked
    session_id: str            # Session identifier
    run_id: str                # Run identifier
    input_message: Message     # User's input message
    messages: list[Message]    # Conversation history
    context: dict[str, Any]    # Additional context
    traceparent: str | None    # W3C Trace Context
    platform_base_url: str | None  # Platform proxy URL
```

### SSEResponse

```python
class SSEResponse:
    def __init__(self, run_id: str | None = None)

    def delta(self, text: str) -> str:
        """Create a streaming text event."""

    def state(self, state: str, detail: dict | None = None) -> str:
        """Create a state change event."""

    def done(
        self,
        final_message: str | None = None,
        usage: Usage | None = None,
    ) -> str:
        """Create a completion event."""

    def error(self, code: str, message: str) -> str:
        """Create an error event."""
```

### PlatformClient

```python
class PlatformClient:
    def __init__(
        self,
        base_url: str,      # Orchestrator URL
        run_id: str,        # Current run ID
        timeout: float = 300.0,
    )

    # Sub-clients
    llm: LLMClient          # LLM proxy (OpenAI-compatible)
    tools: ToolClient       # Tool invocation
    agents: AgentClient     # Agent-to-agent calls
    sessions: SessionClient # Session management
```

## Project Structure

```
sdk/agent/python/
├── pyproject.toml
├── README.md
└── src/
    └── agent_sdk/
        ├── __init__.py    # Package exports
        ├── models.py      # Data models
        ├── sse.py         # SSE utilities
        ├── agent.py       # Agent base class
        └── client.py      # Platform client
```

## License

MIT
