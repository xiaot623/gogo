"""
Pydantic models for agent SDK.

These models define the data structures for communication between
the platform and agents.
"""

from enum import Enum
from typing import Any, Optional

from pydantic import BaseModel, Field


# =============================================================================
# Enums
# =============================================================================


class Role(str, Enum):
    """Message role in a conversation."""

    USER = "user"
    ASSISTANT = "assistant"
    SYSTEM = "system"


class SSEEventType(str, Enum):
    """SSE event types that agents can emit."""

    DELTA = "delta"
    STATE = "state"
    DONE = "done"
    ERROR = "error"


# =============================================================================
# Request Models
# =============================================================================


class Message(BaseModel):
    """A single message in a conversation."""

    role: str = Field(..., description="Message role: user, assistant, or system")
    content: str = Field(..., description="Message content")


class InvokeRequest(BaseModel):
    """
    Request body for the /invoke endpoint.

    This is what the platform sends to the agent when invoking it.
    """

    agent_id: str = Field(..., description="ID of the agent being invoked")
    session_id: str = Field(..., description="Session identifier")
    run_id: str = Field(..., description="Run identifier for this execution")
    input_message: Message = Field(..., description="The user's input message")
    messages: Optional[list[Message]] = Field(
        default=None, description="Full conversation history (transcript)"
    )
    context: Optional[dict[str, Any]] = Field(
        default=None, description="Additional context (user_id, timezone, etc.)"
    )


class InvokeContext(BaseModel):
    """
    Context available to the agent during invocation.

    This is a convenience wrapper that combines request data with headers.
    """

    agent_id: str
    session_id: str
    run_id: str
    input_message: Message
    messages: list[Message] = Field(default_factory=list)
    context: dict[str, Any] = Field(default_factory=dict)

    # Headers from the request
    traceparent: Optional[str] = None
    platform_base_url: Optional[str] = None

    @classmethod
    def from_request(
        cls,
        request: InvokeRequest,
        traceparent: Optional[str] = None,
        platform_base_url: Optional[str] = None,
    ) -> "InvokeContext":
        """Create an InvokeContext from an InvokeRequest and headers."""
        return cls(
            agent_id=request.agent_id,
            session_id=request.session_id,
            run_id=request.run_id,
            input_message=request.input_message,
            messages=request.messages or [],
            context=request.context or {},
            traceparent=traceparent,
            platform_base_url=platform_base_url,
        )


# =============================================================================
# Response Models
# =============================================================================


class HealthResponse(BaseModel):
    """Response for the /health endpoint."""

    status: str = Field(default="healthy", description="Health status")
    version: str = Field(..., description="Agent version")
    capabilities: list[str] = Field(
        default_factory=list, description="List of agent capabilities"
    )


class Usage(BaseModel):
    """Token usage statistics."""

    tokens: Optional[int] = Field(default=None, description="Total tokens used")
    prompt_tokens: Optional[int] = Field(default=None, description="Prompt tokens")
    completion_tokens: Optional[int] = Field(default=None, description="Completion tokens")
    duration_ms: Optional[int] = Field(default=None, description="Duration in milliseconds")


# =============================================================================
# SSE Event Data Models
# =============================================================================


class DeltaEvent(BaseModel):
    """Data for a delta (streaming text) event."""

    text: str = Field(..., description="Text chunk to stream")
    run_id: Optional[str] = Field(default=None, description="Run identifier")


class StateEvent(BaseModel):
    """Data for a state change event."""

    state: str = Field(..., description="New state name")
    detail: Optional[dict[str, Any]] = Field(default=None, description="State details")


class DoneEvent(BaseModel):
    """Data for a done event."""

    final_message: Optional[str] = Field(default=None, description="Complete final message")
    usage: Optional[Usage] = Field(default=None, description="Usage statistics")


class ErrorEvent(BaseModel):
    """Data for an error event."""

    code: str = Field(..., description="Error code")
    message: str = Field(..., description="Error message")


# =============================================================================
# Agent Registration
# =============================================================================


class AgentInfo(BaseModel):
    """Information about an agent for registration."""

    agent_id: str = Field(..., description="Unique agent identifier")
    name: str = Field(..., description="Human-readable agent name")
    endpoint: str = Field(..., description="Agent HTTP endpoint URL")
    capabilities: list[str] = Field(
        default_factory=list, description="List of capability strings"
    )
