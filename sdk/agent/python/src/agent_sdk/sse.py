"""
SSE (Server-Sent Events) utilities for streaming responses.

This module provides helpers to create SSE streams that conform to
the platform's agent protocol.
"""

import json
from collections.abc import AsyncIterator, Iterator
from typing import Any, Optional, Union

from .models import DeltaEvent, DoneEvent, ErrorEvent, SSEEventType, StateEvent, Usage


def format_sse_event(event_type: str, data: Union[dict, str]) -> str:
    """
    Format a single SSE event.

    Args:
        event_type: The event type (delta, done, error, state)
        data: The event data (will be JSON encoded if dict)

    Returns:
        Formatted SSE event string
    """
    if isinstance(data, dict):
        data_str = json.dumps(data, ensure_ascii=False)
    else:
        data_str = data
    return f"event: {event_type}\ndata: {data_str}\n\n"


class SSEResponse:
    """
    Builder for SSE streaming responses.

    This class provides a fluent interface for building SSE streams
    that can be used with FastAPI's StreamingResponse.

    Example:
        ```python
        async def my_handler(ctx: InvokeContext):
            sse = SSEResponse(run_id=ctx.run_id)

            async def generate():
                yield sse.delta("Hello ")
                yield sse.delta("World!")
                yield sse.done(final_message="Hello World!")

            return StreamingResponse(
                generate(),
                media_type="text/event-stream"
            )
        ```
    """

    def __init__(self, run_id: Optional[str] = None):
        """
        Initialize SSE response builder.

        Args:
            run_id: The run ID to include in events
        """
        self.run_id = run_id

    def delta(self, text: str) -> str:
        """
        Create a delta (streaming text) event.

        Args:
            text: The text chunk to stream

        Returns:
            Formatted SSE event string
        """
        event = DeltaEvent(text=text, run_id=self.run_id)
        return format_sse_event(SSEEventType.DELTA.value, event.model_dump(exclude_none=True))

    def state(self, state: str, detail: Optional[dict[str, Any]] = None) -> str:
        """
        Create a state change event.

        Args:
            state: The new state name
            detail: Optional details about the state

        Returns:
            Formatted SSE event string
        """
        event = StateEvent(state=state, detail=detail)
        return format_sse_event(SSEEventType.STATE.value, event.model_dump(exclude_none=True))

    def done(
        self,
        final_message: Optional[str] = None,
        usage: Optional[Usage] = None,
    ) -> str:
        """
        Create a done event.

        Args:
            final_message: The complete final message
            usage: Optional usage statistics

        Returns:
            Formatted SSE event string
        """
        event = DoneEvent(final_message=final_message, usage=usage)
        return format_sse_event(SSEEventType.DONE.value, event.model_dump(exclude_none=True))

    def error(self, code: str, message: str) -> str:
        """
        Create an error event.

        Args:
            code: Error code
            message: Error message

        Returns:
            Formatted SSE event string
        """
        event = ErrorEvent(code=code, message=message)
        return format_sse_event(SSEEventType.ERROR.value, event.model_dump())


async def stream_text(
    text: str,
    run_id: Optional[str] = None,
    chunk_size: int = 10,
    delay_ms: int = 50,
) -> AsyncIterator[str]:
    """
    Stream text as SSE delta events with simulated typing delay.

    This is a convenience function for streaming text character by character
    or in chunks, useful for demos and testing.

    Args:
        text: The text to stream
        run_id: The run ID to include in events
        chunk_size: Number of characters per chunk
        delay_ms: Delay between chunks in milliseconds

    Yields:
        Formatted SSE event strings
    """
    import asyncio

    sse = SSEResponse(run_id=run_id)
    delay_sec = delay_ms / 1000.0

    for i in range(0, len(text), chunk_size):
        chunk = text[i : i + chunk_size]
        yield sse.delta(chunk)
        if delay_ms > 0:
            await asyncio.sleep(delay_sec)

    yield sse.done(final_message=text)


def stream_text_sync(
    text: str,
    run_id: Optional[str] = None,
    chunk_size: int = 10,
) -> Iterator[str]:
    """
    Stream text as SSE delta events (synchronous version).

    Args:
        text: The text to stream
        run_id: The run ID to include in events
        chunk_size: Number of characters per chunk

    Yields:
        Formatted SSE event strings
    """
    sse = SSEResponse(run_id=run_id)

    for i in range(0, len(text), chunk_size):
        chunk = text[i : i + chunk_size]
        yield sse.delta(chunk)

    yield sse.done(final_message=text)
