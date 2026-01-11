import { Hono } from "hono";
import { streamSSE } from "hono/streaming";
import { Gateway } from "./gateway.js";
import { ChatCompletionRequestSchema } from "./types.js";
import { loadConfig } from "./config.js";

// Load config and initialize gateway
const config = loadConfig();

if (Object.keys(config.gateway.providers).length === 0) {
  console.error("Error: No providers configured.");
  console.error("Either create a config.yaml file or set OPENROUTER_API_KEY environment variable.");
  process.exit(1);
}

const gateway = new Gateway(config.gateway);

// Create Hono app
const app = new Hono();

// Health check
app.get("/health", (c) => {
  return c.json({ status: "ok", providers: gateway.listProviders() });
});

// List providers
app.get("/v1/providers", (c) => {
  return c.json({
    providers: gateway.listProviders(),
    default: gateway.getDefaultProvider(),
  });
});

// List models
app.get("/v1/models", async (c) => {
  const providerName = c.req.query("provider");
  try {
    const provider = gateway.getProvider(providerName);
    const models = await provider.listModels();
    return c.json({ object: "list", data: models });
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unknown error";
    return c.json({ error: { message } }, 500);
  }
});

// Chat completions endpoint (OpenAI-compatible)
app.post("/v1/chat/completions", async (c) => {
  const providerName = c.req.query("provider");

  // Parse and validate request
  let body: unknown;
  try {
    body = await c.req.json();
  } catch {
    return c.json({ error: { message: "Invalid JSON body" } }, 400);
  }

  const parsed = ChatCompletionRequestSchema.safeParse(body);
  if (!parsed.success) {
    return c.json({
      error: {
        message: "Invalid request",
        details: parsed.error.errors,
      },
    }, 400);
  }

  const request = parsed.data;

  try {
    const provider = gateway.getProvider(providerName);

    // Handle streaming
    if (request.stream) {
      return streamSSE(c, async (stream) => {
        try {
          for await (const chunk of provider.chatStream(request)) {
            await stream.writeSSE({
              data: JSON.stringify(chunk),
            });
          }
          await stream.writeSSE({ data: "[DONE]" });
        } catch (error) {
          const message = error instanceof Error ? error.message : "Unknown error";
          await stream.writeSSE({
            data: JSON.stringify({ error: { message } }),
          });
        }
      });
    }

    // Non-streaming
    const response = await provider.chat(request);
    return c.json(response);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unknown error";
    return c.json({ error: { message } }, 500);
  }
});

// Start server
const { port, host } = config.server;

console.log(`LLM Gateway starting on http://${host}:${port}`);
console.log(`Providers: ${gateway.listProviders().join(", ")}`);
console.log(`Default provider: ${gateway.getDefaultProvider()}`);

export default {
  port,
  hostname: host,
  fetch: app.fetch,
};
