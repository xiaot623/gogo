import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { writeFileSync, unlinkSync, mkdirSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { loadConfig, loadEnvConfig, buildGatewayConfig } from "../src/config.js";

describe("config", () => {
  let tempDir: string;

  beforeEach(() => {
    // Create a unique temp directory for each test
    tempDir = join(tmpdir(), `llm-gateway-test-${Date.now()}`);
    mkdirSync(tempDir, { recursive: true });
  });

  afterEach(() => {
    // Clean up temp directory
    rmSync(tempDir, { recursive: true, force: true });
    // Clean up env vars
    delete process.env.OPENROUTER_API_KEY;
    delete process.env.OPENROUTER_BASE_URL;
    delete process.env.PORT;
    delete process.env.HOST;
    delete process.env.LLM_GATEWAY_CONFIG;
    delete process.env.TEST_API_KEY;
  });

  describe("loadConfig", () => {
    it("should load config from YAML file", () => {
      const configPath = join(tempDir, "config.yaml");
      writeFileSync(
        configPath,
        `
server:
  port: 8080
  host: localhost

default_provider: openrouter

providers:
  openrouter:
    api_key: test-key-123
    base_url: https://custom.openrouter.ai
`
      );

      const config = loadConfig(tempDir);

      expect(config.server.port).toBe(8080);
      expect(config.server.host).toBe("localhost");
      expect(config.gateway.defaultProvider).toBe("openrouter");
      expect(config.gateway.providers.openrouter).toEqual({
        apiKey: "test-key-123",
        baseUrl: "https://custom.openrouter.ai",
        defaultModel: undefined,
        headers: undefined,
      });
    });

    it("should interpolate environment variables in YAML", () => {
      process.env.TEST_API_KEY = "env-api-key-456";

      const configPath = join(tempDir, "config.yaml");
      writeFileSync(
        configPath,
        `
providers:
  openrouter:
    api_key: \${TEST_API_KEY}
`
      );

      const config = loadConfig(tempDir);

      expect(config.gateway.providers.openrouter.apiKey).toBe("env-api-key-456");
    });

    it("should use default value when env var is not set", () => {
      const configPath = join(tempDir, "config.yaml");
      writeFileSync(
        configPath,
        `
server:
  port: \${PORT:-9000}
providers:
  openrouter:
    api_key: \${MISSING_KEY:-default-key}
`
      );

      const config = loadConfig(tempDir);

      expect(config.server.port).toBe(9000);
      expect(config.gateway.providers.openrouter.apiKey).toBe("default-key");
    });

    it("should return empty string when env var not set and no default", () => {
      const configPath = join(tempDir, "config.yaml");
      writeFileSync(
        configPath,
        `
providers:
  openrouter:
    api_key: \${NONEXISTENT_VAR}
`
      );

      const config = loadConfig(tempDir);

      expect(config.gateway.providers.openrouter.apiKey).toBe("");
    });

    it("should fall back to env vars when no config file exists", () => {
      process.env.OPENROUTER_API_KEY = "env-key";
      process.env.PORT = "4000";
      process.env.HOST = "127.0.0.1";

      const config = loadConfig(tempDir);

      expect(config.server.port).toBe(4000);
      expect(config.server.host).toBe("127.0.0.1");
      expect(config.gateway.providers.openrouter?.apiKey).toBe("env-key");
      expect(config.gateway.defaultProvider).toBe("openrouter");
    });

    it("should use defaults when no config file and no env vars", () => {
      const config = loadConfig(tempDir);

      expect(config.server.port).toBe(3000);
      expect(config.server.host).toBe("0.0.0.0");
      expect(config.gateway.providers).toEqual({});
    });

    it("should load config from LLM_GATEWAY_CONFIG env path", () => {
      const customPath = join(tempDir, "custom-config.yaml");
      writeFileSync(
        customPath,
        `
server:
  port: 7777
providers:
  openrouter:
    api_key: custom-key
`
      );

      process.env.LLM_GATEWAY_CONFIG = customPath;

      const config = loadConfig(tempDir);

      expect(config.server.port).toBe(7777);
      expect(config.gateway.providers.openrouter.apiKey).toBe("custom-key");
    });

    it("should try alternative config file names", () => {
      // Test config.yml
      const configPath = join(tempDir, "config.yml");
      writeFileSync(
        configPath,
        `
server:
  port: 5555
providers:
  openrouter:
    api_key: yml-key
`
      );

      const config = loadConfig(tempDir);

      expect(config.server.port).toBe(5555);
      expect(config.gateway.providers.openrouter.apiKey).toBe("yml-key");
    });

    it("should handle provider with custom headers", () => {
      const configPath = join(tempDir, "config.yaml");
      writeFileSync(
        configPath,
        `
providers:
  openrouter:
    api_key: test-key
    headers:
      X-Custom-Header: custom-value
      X-Another: another-value
`
      );

      const config = loadConfig(tempDir);

      expect(config.gateway.providers.openrouter.headers).toEqual({
        "X-Custom-Header": "custom-value",
        "X-Another": "another-value",
      });
    });
  });

  describe("loadEnvConfig (legacy)", () => {
    it("should load config from environment variables", () => {
      process.env.PORT = "8888";
      process.env.HOST = "192.168.1.1";
      process.env.OPENROUTER_API_KEY = "legacy-key";

      const config = loadEnvConfig();

      expect(config.port).toBe(8888);
      expect(config.host).toBe("192.168.1.1");
      expect(config.openrouterApiKey).toBe("legacy-key");
    });

    it("should use defaults when env vars not set", () => {
      const config = loadEnvConfig();

      expect(config.port).toBe(3000);
      expect(config.host).toBe("0.0.0.0");
      expect(config.openrouterApiKey).toBeUndefined();
    });
  });

  describe("buildGatewayConfig (legacy)", () => {
    it("should build gateway config from env config", () => {
      const envConfig = {
        port: 3000,
        host: "0.0.0.0",
        openrouterApiKey: "test-key",
      };

      const gatewayConfig = buildGatewayConfig(envConfig);

      expect(gatewayConfig.providers.openrouter).toEqual({
        apiKey: "test-key",
      });
      expect(gatewayConfig.defaultProvider).toBe("openrouter");
    });

    it("should return empty providers when no API key", () => {
      const envConfig = {
        port: 3000,
        host: "0.0.0.0",
      };

      const gatewayConfig = buildGatewayConfig(envConfig);

      expect(gatewayConfig.providers).toEqual({});
      expect(gatewayConfig.defaultProvider).toBeUndefined();
    });
  });
});
