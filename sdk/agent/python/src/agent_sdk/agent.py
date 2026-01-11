"""
Agent base class and application builder.

This module provides the core abstractions for building agents:
- Agent: Base class for implementing agent logic
- AgentApp: FastAPI application wrapper with standard endpoints
"""

import logging
from abc import ABC, abstractmethod
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any, Callable, Optional

from fastapi import FastAPI, Request
from fastapi.responses import StreamingResponse

from .models import AgentInfo, HealthResponse, InvokeContext, InvokeRequest
from .sse import SSEResponse

logger = logging.getLogger(__name__)


class Agent(ABC):
    """
    Abstract base class for implementing agents.

    Subclass this to implement your agent logic. You only need to implement
    the `invoke` method which handles incoming requests and yields SSE events.

    Example:
        ```python
        class EchoAgent(Agent):
            async def invoke(self, ctx: InvokeContext) -> AsyncIterator[str]:
                sse = SSEResponse(run_id=ctx.run_id)
                text = f"You said: {ctx.input_message.content}"

                for char in text:
                    yield sse.delta(char)
                    await asyncio.sleep(0.05)

                yield sse.done(final_message=text)

        # Create and run the agent
        agent = EchoAgent(
            agent_id="echo",
            name="Echo Agent",
            version="1.0.0",
        )
        agent.run(port=8000)
        ```
    """

    def __init__(
        self,
        agent_id: str,
        name: str,
        version: str = "0.1.0",
        capabilities: Optional[list[str]] = None,
    ):
        """
        Initialize the agent.

        Args:
            agent_id: Unique identifier for this agent
            name: Human-readable name
            version: Agent version string
            capabilities: List of capability strings
        """
        self.agent_id = agent_id
        self.name = name
        self.version = version
        self.capabilities = capabilities or []
        self._app: Optional[FastAPI] = None

    @abstractmethod
    async def invoke(self, ctx: InvokeContext) -> AsyncIterator[str]:
        """
        Handle an invocation request.

        This is the main method you need to implement. It receives the
        invocation context and should yield SSE-formatted event strings.

        Use the SSEResponse helper to create properly formatted events:

        ```python
        async def invoke(self, ctx: InvokeContext) -> AsyncIterator[str]:
            sse = SSEResponse(run_id=ctx.run_id)
            yield sse.delta("Hello")
            yield sse.done(final_message="Hello")
        ```

        Args:
            ctx: The invocation context containing the request data

        Yields:
            SSE-formatted event strings
        """
        yield ""  # Make this a generator for type checking

    async def on_startup(self) -> None:
        """
        Called when the agent starts up.

        Override this to perform initialization tasks like loading models,
        connecting to databases, etc.
        """
        pass

    async def on_shutdown(self) -> None:
        """
        Called when the agent shuts down.

        Override this to perform cleanup tasks.
        """
        pass

    def get_info(self) -> AgentInfo:
        """Get agent registration info."""
        return AgentInfo(
            agent_id=self.agent_id,
            name=self.name,
            endpoint="",  # Will be set during registration
            capabilities=self.capabilities,
        )

    def get_health(self) -> HealthResponse:
        """Get agent health status."""
        return HealthResponse(
            status="healthy",
            version=self.version,
            capabilities=self.capabilities,
        )

    def create_app(self) -> FastAPI:
        """
        Create a FastAPI application for this agent.

        Returns:
            Configured FastAPI application
        """
        if self._app is not None:
            return self._app

        @asynccontextmanager
        async def lifespan(app: FastAPI):
            logger.info(f"Agent '{self.name}' starting up...")
            await self.on_startup()
            yield
            logger.info(f"Agent '{self.name}' shutting down...")
            await self.on_shutdown()

        app = FastAPI(
            title=self.name,
            description=f"Agent: {self.name}",
            version=self.version,
            lifespan=lifespan,
        )

        @app.get("/health", response_model=HealthResponse)
        async def health_check():
            return self.get_health()

        @app.post("/invoke")
        async def invoke_handler(request: Request, body: InvokeRequest):
            # Extract headers
            traceparent = request.headers.get("traceparent")
            platform_base_url = request.headers.get("x-platform-base-url")

            # Also check X- prefixed headers
            session_id = request.headers.get("X-Session-ID", body.session_id)
            run_id = request.headers.get("X-Run-ID", body.run_id)

            # Update body with header values if present
            if session_id != body.session_id:
                body.session_id = session_id
            if run_id != body.run_id:
                body.run_id = run_id

            # Create context
            ctx = InvokeContext.from_request(
                body,
                traceparent=traceparent,
                platform_base_url=platform_base_url,
            )

            logger.info(f"[invoke] session_id={ctx.session_id}, run_id={ctx.run_id}")
            logger.debug(f"[invoke] input: {ctx.input_message.content}")

            return StreamingResponse(
                self.invoke(ctx),
                media_type="text/event-stream",
                headers={
                    "Cache-Control": "no-cache",
                    "Connection": "keep-alive",
                    "X-Accel-Buffering": "no",
                },
            )

        @app.get("/")
        async def root():
            return {
                "agent_id": self.agent_id,
                "name": self.name,
                "version": self.version,
                "capabilities": self.capabilities,
                "endpoints": {
                    "/health": "Health check (GET)",
                    "/invoke": "Agent invocation (POST)",
                },
            }

        self._app = app
        return app

    def run(
        self,
        host: str = "0.0.0.0",
        port: int = 8000,
        log_level: str = "info",
        **kwargs: Any,
    ) -> None:
        """
        Run the agent server.

        Args:
            host: Host to bind to
            port: Port to listen on
            log_level: Logging level
            **kwargs: Additional uvicorn arguments
        """
        import uvicorn

        app = self.create_app()
        uvicorn.run(app, host=host, port=port, log_level=log_level, **kwargs)


class FunctionAgent(Agent):
    """
    A simple agent that wraps a function.

    This is useful for simple agents that don't need the full class structure.

    Example:
        ```python
        async def my_handler(ctx: InvokeContext) -> AsyncIterator[str]:
            sse = SSEResponse(run_id=ctx.run_id)
            yield sse.delta("Hello!")
            yield sse.done(final_message="Hello!")

        agent = FunctionAgent(
            agent_id="simple",
            name="Simple Agent",
            handler=my_handler,
        )
        agent.run()
        ```
    """

    def __init__(
        self,
        agent_id: str,
        name: str,
        handler: Callable[[InvokeContext], AsyncIterator[str]],
        version: str = "0.1.0",
        capabilities: Optional[list[str]] = None,
    ):
        super().__init__(agent_id, name, version, capabilities)
        self._handler = handler

    async def invoke(self, ctx: InvokeContext) -> AsyncIterator[str]:
        async for event in self._handler(ctx):
            yield event


def create_agent(
    agent_id: str,
    name: str,
    version: str = "0.1.0",
    capabilities: Optional[list[str]] = None,
) -> Callable[[Callable[[InvokeContext], AsyncIterator[str]]], FunctionAgent]:
    """
    Decorator to create an agent from a handler function.

    Example:
        ```python
        @create_agent("echo", "Echo Agent")
        async def echo_agent(ctx: InvokeContext) -> AsyncIterator[str]:
            sse = SSEResponse(run_id=ctx.run_id)
            yield sse.delta(ctx.input_message.content)
            yield sse.done()

        # Run the agent
        echo_agent.run(port=8000)
        ```

    Args:
        agent_id: Unique identifier for this agent
        name: Human-readable name
        version: Agent version string
        capabilities: List of capability strings

    Returns:
        Decorator that creates a FunctionAgent
    """

    def decorator(
        handler: Callable[[InvokeContext], AsyncIterator[str]],
    ) -> FunctionAgent:
        return FunctionAgent(
            agent_id=agent_id,
            name=name,
            handler=handler,
            version=version,
            capabilities=capabilities,
        )

    return decorator
