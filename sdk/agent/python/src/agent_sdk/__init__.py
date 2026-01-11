"""
Agent SDK for the Multi-Agent Platform.

This SDK provides tools for building agents that integrate with the platform:

- Agent base class for implementing agent logic
- SSE streaming utilities
- Platform client for LLM/Tool/Agent calls

Quick Start:
    ```python
    from agent_sdk import Agent, InvokeContext, SSEResponse

    class MyAgent(Agent):
        async def invoke(self, ctx: InvokeContext):
            sse = SSEResponse(run_id=ctx.run_id)
            yield sse.delta("Hello!")
            yield sse.done(final_message="Hello!")

    agent = MyAgent(agent_id="my-agent", name="My Agent")
    agent.run(port=8000)
    ```

Using the decorator:
    ```python
    from agent_sdk import create_agent, InvokeContext, SSEResponse

    @create_agent("echo", "Echo Agent")
    async def echo_agent(ctx: InvokeContext):
        sse = SSEResponse(run_id=ctx.run_id)
        yield sse.delta(ctx.input_message.content)
        yield sse.done()

    echo_agent.run(port=8000)
    ```
"""

from .agent import Agent, FunctionAgent, create_agent
from .client import (
    AgentClient,
    LLMClient,
    PlatformClient,
    PlatformError,
    SessionClient,
    ToolClient,
    ToolResult,
)
from .models import (
    AgentInfo,
    DeltaEvent,
    DoneEvent,
    ErrorEvent,
    HealthResponse,
    InvokeContext,
    InvokeRequest,
    Message,
    Role,
    SSEEventType,
    StateEvent,
    Usage,
)
from .sse import SSEResponse, format_sse_event, stream_text, stream_text_sync

__version__ = "0.1.0"

__all__ = [
    # Agent
    "Agent",
    "FunctionAgent",
    "create_agent",
    # Client
    "PlatformClient",
    "PlatformError",
    "LLMClient",
    "ToolClient",
    "ToolResult",
    "AgentClient",
    "SessionClient",
    # Models
    "Message",
    "InvokeRequest",
    "InvokeContext",
    "HealthResponse",
    "Usage",
    "AgentInfo",
    "Role",
    "SSEEventType",
    # SSE Events
    "DeltaEvent",
    "DoneEvent",
    "ErrorEvent",
    "StateEvent",
    # SSE Utilities
    "SSEResponse",
    "format_sse_event",
    "stream_text",
    "stream_text_sync",
]
