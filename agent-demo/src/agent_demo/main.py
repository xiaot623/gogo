"""
Demo agent using the agent-sdk.

This is a mock implementation that simulates calling the llmproxy streaming interface
and prints the SSE response format.
"""

import asyncio
import json
import time
import uuid
from collections.abc import AsyncIterator

from agent_sdk import Agent, InvokeContext, SSEResponse, Usage


class DemoAgent(Agent):
    """
    A mock agent that simulates the llmproxy streaming SSE response.
    """

    def __init__(self):
        super().__init__(
            agent_id="demo",
            name="Demo Agent",
            version="0.1.0",
            capabilities=["streaming", "llm-mock"],
        )

    async def invoke(self, ctx: InvokeContext) -> AsyncIterator[str]:
        """Handle an invocation request, simulating llmproxy SSE streaming."""
        # Print the incoming request
        print("\n" + "=" * 60)
        print("INCOMING REQUEST:")
        print(f"  run_id: {ctx.run_id}")
        print(f"  input: {ctx.input_message.content}")
        print("=" * 60)

        sse = SSEResponse(run_id=ctx.run_id)
        user_input = ctx.input_message.content

        # Mock response content
        response_content = f"This is a mock LLM response for: {user_input}"
        chunk_id = f"chatcmpl-{uuid.uuid4().hex[:8]}"
        created = int(time.time())
        model = "mock-gpt-4"

        print("\n[Mock llmproxy /v1/chat/completions SSE stream]")
        print("-" * 60)

        # First chunk with role
        first_chunk = {
            "id": chunk_id,
            "object": "chat.completion.chunk",
            "created": created,
            "model": model,
            "choices": [{"index": 0, "delta": {"role": "assistant", "content": ""}, "finish_reason": None}],
        }
        print(f"data: {json.dumps(first_chunk)}\n")

        # Stream content chunks
        chunk_size = 5
        delay = 0.03
        full_text = ""

        for i in range(0, len(response_content), chunk_size):
            chunk_text = response_content[i : i + chunk_size]
            full_text += chunk_text

            stream_chunk = {
                "id": chunk_id,
                "object": "chat.completion.chunk",
                "created": created,
                "model": model,
                "choices": [{"index": 0, "delta": {"content": chunk_text}, "finish_reason": None}],
            }
            print(f"data: {json.dumps(stream_chunk)}\n")
            yield sse.delta(chunk_text)
            await asyncio.sleep(delay)

        # Final chunk with finish_reason
        final_chunk = {
            "id": chunk_id,
            "object": "chat.completion.chunk",
            "created": created,
            "model": model,
            "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
        }
        print(f"data: {json.dumps(final_chunk)}\n")
        print("data: [DONE]\n")
        print("-" * 60)

        # Usage stats
        prompt_tokens = len(user_input.split())
        completion_tokens = len(response_content.split())
        usage = Usage(
            tokens=prompt_tokens + completion_tokens,
            prompt_tokens=prompt_tokens,
            completion_tokens=completion_tokens,
            duration_ms=int((len(response_content) / chunk_size) * delay * 1000),
        )
        yield sse.done(final_message=full_text, usage=usage)


# Create the agent instance
agent = DemoAgent()

# Create the FastAPI app (for use with uvicorn directly)
app = agent.create_app()


def run():
    """Run the agent server."""
    agent.run(port=8000)


if __name__ == "__main__":
    run()
