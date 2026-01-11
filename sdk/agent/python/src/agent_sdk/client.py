"""
Platform client for agents to interact with the orchestrator.

This module provides clients for:
- LLM proxy (OpenAI-compatible)
- Tool invocation
- Agent-to-agent calls
- Session management
"""

import json
import logging
from collections.abc import AsyncIterator
from typing import Any, Optional

import httpx

from .models import Message

logger = logging.getLogger(__name__)


class PlatformError(Exception):
    """Base exception for platform errors."""

    def __init__(self, code: str, message: str):
        self.code = code
        self.message = message
        super().__init__(f"{code}: {message}")


class ToolResult:
    """Result of a tool invocation."""

    def __init__(
        self,
        tool_call_id: str,
        status: str,
        result: Optional[dict[str, Any]] = None,
        error: Optional[dict[str, Any]] = None,
    ):
        self.tool_call_id = tool_call_id
        self.status = status
        self.result = result
        self.error = error

    @property
    def succeeded(self) -> bool:
        return self.status == "succeeded"

    @property
    def pending(self) -> bool:
        return self.status == "pending"

    @property
    def failed(self) -> bool:
        return self.status == "failed"


class LLMClient:
    """
    OpenAI-compatible LLM client that routes through the platform.

    This client provides an interface similar to the OpenAI Python SDK,
    but routes requests through the platform's LLM proxy for tracing.

    Example:
        ```python
        client = PlatformClient(base_url="http://orchestrator:8080", run_id="run_001")

        # Non-streaming
        response = await client.llm.chat_completions(
            model="gpt-4",
            messages=[{"role": "user", "content": "Hello"}]
        )
        print(response["choices"][0]["message"]["content"])

        # Streaming
        async for chunk in client.llm.chat_completions_stream(
            model="gpt-4",
            messages=[{"role": "user", "content": "Hello"}]
        ):
            print(chunk["choices"][0]["delta"].get("content", ""), end="")
        ```
    """

    def __init__(self, http_client: httpx.AsyncClient, run_id: str):
        self._client = http_client
        self._run_id = run_id

    async def chat_completions(
        self,
        model: str,
        messages: list[dict[str, str]],
        **kwargs: Any,
    ) -> dict[str, Any]:
        """
        Create a chat completion (non-streaming).

        Args:
            model: Model name
            messages: List of messages
            **kwargs: Additional OpenAI parameters

        Returns:
            OpenAI-compatible response
        """
        payload = {
            "model": model,
            "messages": messages,
            "stream": False,
            **kwargs,
        }

        response = await self._client.post(
            "/v1/chat/completions",
            json=payload,
            headers={"x-run-id": self._run_id},
        )
        response.raise_for_status()
        return response.json()

    async def chat_completions_stream(
        self,
        model: str,
        messages: list[dict[str, str]],
        **kwargs: Any,
    ) -> AsyncIterator[dict[str, Any]]:
        """
        Create a streaming chat completion.

        Args:
            model: Model name
            messages: List of messages
            **kwargs: Additional OpenAI parameters

        Yields:
            OpenAI-compatible streaming chunks
        """
        payload = {
            "model": model,
            "messages": messages,
            "stream": True,
            **kwargs,
        }

        async with self._client.stream(
            "POST",
            "/v1/chat/completions",
            json=payload,
            headers={"x-run-id": self._run_id},
        ) as response:
            response.raise_for_status()
            async for line in response.aiter_lines():
                if line.startswith("data: "):
                    data = line[6:]
                    if data == "[DONE]":
                        break
                    yield json.loads(data)


class ToolClient:
    """
    Client for invoking tools through the platform.

    The platform handles tool routing, approval workflows, and
    client tool delegation.

    Example:
        ```python
        client = PlatformClient(base_url="http://orchestrator:8080", run_id="run_001")

        # Invoke a tool
        result = await client.tools.invoke(
            tool_name="weather.query",
            args={"city": "Beijing"}
        )

        if result.succeeded:
            print(result.result)
        elif result.pending:
            # Tool requires approval, wait for it
            result = await client.tools.wait(result.tool_call_id)
        ```
    """

    def __init__(self, http_client: httpx.AsyncClient, run_id: str):
        self._client = http_client
        self._run_id = run_id

    async def invoke(
        self,
        tool_name: str,
        args: dict[str, Any],
        idempotency_key: Optional[str] = None,
        timeout_ms: int = 60000,
    ) -> ToolResult:
        """
        Invoke a tool.

        Args:
            tool_name: Name of the tool to invoke
            args: Tool arguments
            idempotency_key: Optional idempotency key
            timeout_ms: Timeout in milliseconds

        Returns:
            ToolResult with status and result/error
        """
        payload = {
            "run_id": self._run_id,
            "args": args,
            "timeout_ms": timeout_ms,
        }
        if idempotency_key:
            payload["idempotency_key"] = idempotency_key

        response = await self._client.post(
            f"/v1/tools/{tool_name}:invoke",
            json=payload,
        )
        response.raise_for_status()
        data = response.json()

        return ToolResult(
            tool_call_id=data["tool_call_id"],
            status=data["status"],
            result=data.get("result"),
            error=data.get("error"),
        )

    async def get_status(self, tool_call_id: str) -> ToolResult:
        """
        Get the status of a tool call.

        Args:
            tool_call_id: The tool call ID

        Returns:
            ToolResult with current status
        """
        response = await self._client.get(f"/v1/tool_calls/{tool_call_id}")
        response.raise_for_status()
        data = response.json()

        return ToolResult(
            tool_call_id=data["tool_call_id"],
            status=data["status"],
            result=data.get("result"),
            error=data.get("error"),
        )

    async def wait(
        self,
        tool_call_id: str,
        timeout_ms: int = 60000,
    ) -> ToolResult:
        """
        Wait for a tool call to complete.

        Args:
            tool_call_id: The tool call ID
            timeout_ms: Maximum wait time in milliseconds

        Returns:
            ToolResult with final status
        """
        response = await self._client.post(
            f"/v1/tool_calls/{tool_call_id}:wait",
            params={"timeout_ms": timeout_ms},
        )
        response.raise_for_status()
        data = response.json()

        return ToolResult(
            tool_call_id=data["tool_call_id"],
            status=data["status"],
            result=data.get("result"),
            error=data.get("error"),
        )

    async def invoke_and_wait(
        self,
        tool_name: str,
        args: dict[str, Any],
        idempotency_key: Optional[str] = None,
        timeout_ms: int = 60000,
    ) -> ToolResult:
        """
        Invoke a tool and wait for completion.

        This is a convenience method that combines invoke + wait.

        Args:
            tool_name: Name of the tool to invoke
            args: Tool arguments
            idempotency_key: Optional idempotency key
            timeout_ms: Timeout in milliseconds

        Returns:
            ToolResult with final status
        """
        result = await self.invoke(tool_name, args, idempotency_key, timeout_ms)
        if result.pending:
            result = await self.wait(result.tool_call_id, timeout_ms)
        return result


class AgentClient:
    """
    Client for agent-to-agent calls.

    Example:
        ```python
        client = PlatformClient(base_url="http://orchestrator:8080", run_id="run_001")

        # Invoke another agent
        async for event in client.agents.invoke(
            agent_id="weather_agent",
            message={"role": "user", "content": "What's the weather?"}
        ):
            if event["type"] == "delta":
                print(event["data"]["text"], end="")
        ```
    """

    def __init__(self, http_client: httpx.AsyncClient, run_id: str):
        self._client = http_client
        self._run_id = run_id

    async def invoke(
        self,
        agent_id: str,
        message: dict[str, str],
    ) -> AsyncIterator[dict[str, Any]]:
        """
        Invoke another agent and stream the response.

        Args:
            agent_id: ID of the agent to invoke
            message: Input message (role and content)

        Yields:
            SSE events from the child agent
        """
        payload = {
            "parent_run_id": self._run_id,
            "input_message": message,
        }

        async with self._client.stream(
            "POST",
            f"/v1/agents/{agent_id}:invoke",
            json=payload,
        ) as response:
            response.raise_for_status()

            event_type = None
            async for line in response.aiter_lines():
                line = line.strip()
                if not line:
                    continue
                if line.startswith("event:"):
                    event_type = line[6:].strip()
                elif line.startswith("data:"):
                    data = json.loads(line[5:].strip())
                    yield {"type": event_type, "data": data}

    async def list_agents(self) -> list[dict[str, Any]]:
        """
        List all registered agents.

        Returns:
            List of agent information
        """
        response = await self._client.get("/v1/agents")
        response.raise_for_status()
        return response.json()["agents"]


class SessionClient:
    """
    Client for session management.

    Example:
        ```python
        client = PlatformClient(base_url="http://orchestrator:8080", run_id="run_001")

        # Get session messages
        messages = await client.sessions.get_messages("sess_001")
        for msg in messages:
            print(f"{msg.role}: {msg.content}")
        ```
    """

    def __init__(self, http_client: httpx.AsyncClient):
        self._client = http_client

    async def get_messages(
        self,
        session_id: str,
        limit: int = 50,
        before: Optional[str] = None,
    ) -> list[Message]:
        """
        Get messages for a session.

        Args:
            session_id: Session identifier
            limit: Maximum number of messages
            before: Cursor for pagination

        Returns:
            List of messages
        """
        params = {"limit": limit}
        if before:
            params["before"] = before

        response = await self._client.get(
            f"/v1/sessions/{session_id}/messages",
            params=params,
        )
        response.raise_for_status()
        data = response.json()

        return [Message(**msg) for msg in data["messages"]]


class PlatformClient:
    """
    Main client for interacting with the platform from an agent.

    This client provides access to all platform services:
    - LLM proxy (OpenAI-compatible)
    - Tool invocation
    - Agent-to-agent calls
    - Session management

    Example:
        ```python
        async def my_agent(ctx: InvokeContext):
            client = PlatformClient(
                base_url=ctx.platform_base_url or "http://orchestrator:8080",
                run_id=ctx.run_id,
            )

            # Use LLM
            response = await client.llm.chat_completions(
                model="gpt-4",
                messages=[{"role": "user", "content": ctx.input_message.content}]
            )

            # Use tools
            result = await client.tools.invoke_and_wait(
                "weather.query",
                {"city": "Beijing"}
            )
        ```
    """

    def __init__(
        self,
        base_url: str,
        run_id: str,
        timeout: float = 300.0,
    ):
        """
        Initialize the platform client.

        Args:
            base_url: Platform orchestrator URL
            run_id: Current run ID (for tracing)
            timeout: HTTP timeout in seconds
        """
        self._base_url = base_url.rstrip("/")
        self._run_id = run_id
        self._client = httpx.AsyncClient(
            base_url=self._base_url,
            timeout=timeout,
        )

        # Initialize sub-clients
        self.llm = LLMClient(self._client, run_id)
        self.tools = ToolClient(self._client, run_id)
        self.agents = AgentClient(self._client, run_id)
        self.sessions = SessionClient(self._client)

    async def close(self) -> None:
        """Close the HTTP client."""
        await self._client.aclose()

    async def __aenter__(self) -> "PlatformClient":
        return self

    async def __aexit__(self, *args: Any) -> None:
        await self.close()

    async def register_agent(
        self,
        agent_id: str,
        name: str,
        endpoint: str,
        capabilities: Optional[list[str]] = None,
    ) -> dict[str, Any]:
        """
        Register an agent with the platform.

        Args:
            agent_id: Unique agent identifier
            name: Human-readable name
            endpoint: Agent HTTP endpoint URL
            capabilities: List of capability strings

        Returns:
            Registration response
        """
        payload = {
            "agent_id": agent_id,
            "name": name,
            "endpoint": endpoint,
            "capabilities": capabilities or [],
        }

        response = await self._client.post("/v1/agents/register", json=payload)
        response.raise_for_status()
        return response.json()
