"""
Demo agent using the agent-sdk.

This is a simple example showing how to use the SDK to build an agent.
"""

import asyncio
from collections.abc import AsyncIterator

from agent_sdk import Agent, InvokeContext, SSEResponse, Usage


class DemoAgent(Agent):
    """
    A demo agent that echoes user input with mock responses.

    This demonstrates:
    - Subclassing Agent
    - Using SSEResponse for streaming
    - Implementing the invoke method
    """

    def __init__(self):
        super().__init__(
            agent_id="demo",
            name="Demo Agent",
            version="0.1.0",
            capabilities=["streaming", "echo"],
        )
        self._responses = {
            "hello": "Hello! I'm the demo agent. How can I help you today?",
            "weather": "The weather today is sunny with a temperature of 25C. Perfect for outdoor activities!",
            "help": "I'm a demo agent that can help you test the multi-agent platform. Try asking me about the weather or just say hello!",
        }

    def _generate_response(self, user_input: str) -> str:
        """Generate a mock response based on user input."""
        lower_input = user_input.lower()
        for keyword, response in self._responses.items():
            if keyword in lower_input:
                return response

        return f'I received your message: "{user_input}". This is a demo response from the agent.'

    async def invoke(self, ctx: InvokeContext) -> AsyncIterator[str]:
        """Handle an invocation request with streaming response."""
        sse = SSEResponse(run_id=ctx.run_id)

        # Generate response
        response_text = self._generate_response(ctx.input_message.content)

        # Stream the response character by character
        chunk_size = 5
        delay = 0.05

        for i in range(0, len(response_text), chunk_size):
            chunk = response_text[i : i + chunk_size]
            yield sse.delta(chunk)
            await asyncio.sleep(delay)

        # Send done event with usage stats
        usage = Usage(
            tokens=len(response_text.split()),
            prompt_tokens=len(ctx.input_message.content.split()),
            completion_tokens=len(response_text.split()),
            duration_ms=int((len(response_text) / chunk_size) * delay * 1000),
        )
        yield sse.done(final_message=response_text, usage=usage)


# Create the agent instance
agent = DemoAgent()

# Create the FastAPI app (for use with uvicorn directly)
app = agent.create_app()


def run():
    """Run the agent server."""
    agent.run(port=8000)


if __name__ == "__main__":
    run()
