import { readFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { parse as parseYaml } from "yaml";
import { z } from "zod";
import type { GatewayConfig, ProviderConfig } from "./types.js";

// ============================================================================
// Config Schema
// ============================================================================

const ProviderConfigSchema = z.object({
  api_key: z.string().optional(),
  base_url: z.string().optional(),
  default_model: z.string().optional(),
  headers: z.record(z.string()).optional(),
});

const ServerConfigSchema = z.object({
  port: z.coerce.number().default(3000),
  host: z.string().default("0.0.0.0"),
});

const YamlConfigSchema = z.object({
  server: ServerConfigSchema.optional(),
  default_provider: z.string().optional(),
  providers: z.record(ProviderConfigSchema).optional(),
});

export type YamlConfig = z.infer<typeof YamlConfigSchema>;

export interface AppConfig {
  server: {
    port: number;
    host: string;
  };
  gateway: GatewayConfig;
}

// ============================================================================
// Environment Variable Interpolation
// ============================================================================

/**
 * Interpolate environment variables in a string.
 * Supports ${VAR} and ${VAR:-default} syntax.
 */
function interpolateEnv(value: string): string {
  return value.replace(/\$\{([^}]+)\}/g, (_, expr: string) => {
    const [varName, defaultValue] = expr.split(":-");
    const envValue = process.env[varName.trim()];
    if (envValue !== undefined) {
      return envValue;
    }
    if (defaultValue !== undefined) {
      return defaultValue;
    }
    return "";
  });
}

/**
 * Recursively interpolate environment variables in an object.
 */
function interpolateEnvInObject<T>(obj: T): T {
  if (typeof obj === "string") {
    return interpolateEnv(obj) as T;
  }
  if (Array.isArray(obj)) {
    return obj.map(interpolateEnvInObject) as T;
  }
  if (obj !== null && typeof obj === "object") {
    const result: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(obj)) {
      result[key] = interpolateEnvInObject(value);
    }
    return result as T;
  }
  return obj;
}

// ============================================================================
// Config Loading
// ============================================================================

const CONFIG_PATHS = [
  "config.yaml",
  "config.yml",
  "llm-gateway.yaml",
  "llm-gateway.yml",
];

/**
 * Find config file path.
 */
function findConfigPath(basePath?: string): string | null {
  const base = basePath ?? process.cwd();
  
  // Check explicit path from env
  const envPath = process.env.LLM_GATEWAY_CONFIG;
  if (envPath) {
    const resolved = resolve(base, envPath);
    if (existsSync(resolved)) {
      return resolved;
    }
    console.warn(`Config file not found at ${resolved}`);
  }

  // Check default paths
  for (const configPath of CONFIG_PATHS) {
    const resolved = resolve(base, configPath);
    if (existsSync(resolved)) {
      return resolved;
    }
  }

  return null;
}

/**
 * Load and parse YAML config file.
 */
function loadYamlConfig(configPath: string): YamlConfig {
  const content = readFileSync(configPath, "utf-8");
  const rawConfig = parseYaml(content);
  const interpolated = interpolateEnvInObject(rawConfig);
  return YamlConfigSchema.parse(interpolated);
}

/**
 * Convert YAML provider config to internal format.
 */
function convertProviderConfig(yaml: z.infer<typeof ProviderConfigSchema>): ProviderConfig {
  return {
    apiKey: yaml.api_key ?? "",
    baseUrl: yaml.base_url,
    defaultModel: yaml.default_model,
    headers: yaml.headers,
  };
}

/**
 * Build AppConfig from YAML config.
 */
function buildFromYaml(yaml: YamlConfig): AppConfig {
  const providers: Record<string, ProviderConfig> = {};

  if (yaml.providers) {
    for (const [name, providerYaml] of Object.entries(yaml.providers)) {
      providers[name] = convertProviderConfig(providerYaml);
    }
  }

  return {
    server: {
      port: yaml.server?.port ?? 3000,
      host: yaml.server?.host ?? "0.0.0.0",
    },
    gateway: {
      providers,
      defaultProvider: yaml.default_provider,
    },
  };
}

/**
 * Build AppConfig from environment variables only.
 */
function buildFromEnv(): AppConfig {
  const providers: Record<string, ProviderConfig> = {};

  // OpenRouter from env
  const openrouterKey = process.env.OPENROUTER_API_KEY;
  if (openrouterKey) {
    providers.openrouter = {
      apiKey: openrouterKey,
      baseUrl: process.env.OPENROUTER_BASE_URL,
    };
  }

  return {
    server: {
      port: parseInt(process.env.PORT ?? "3000", 10),
      host: process.env.HOST ?? "0.0.0.0",
    },
    gateway: {
      providers,
      defaultProvider: Object.keys(providers)[0],
    },
  };
}

/**
 * Load configuration from YAML file or environment variables.
 * 
 * Config file locations (in order of priority):
 * 1. LLM_GATEWAY_CONFIG environment variable
 * 2. config.yaml / config.yml
 * 3. llm-gateway.yaml / llm-gateway.yml
 * 
 * Falls back to environment variables if no config file found.
 */
export function loadConfig(basePath?: string): AppConfig {
  const configPath = findConfigPath(basePath);

  if (configPath) {
    console.log(`Loading config from ${configPath}`);
    const yaml = loadYamlConfig(configPath);
    return buildFromYaml(yaml);
  }

  console.log("No config file found, using environment variables");
  return buildFromEnv();
}

// ============================================================================
// Legacy exports (for backwards compatibility)
// ============================================================================

export interface EnvConfig {
  port: number;
  host: string;
  openrouterApiKey?: string;
}

export function loadEnvConfig(): EnvConfig {
  return {
    port: parseInt(process.env.PORT ?? "3000", 10),
    host: process.env.HOST ?? "0.0.0.0",
    openrouterApiKey: process.env.OPENROUTER_API_KEY,
  };
}

export function buildGatewayConfig(env: EnvConfig): GatewayConfig {
  const config: GatewayConfig = {
    providers: {},
  };

  if (env.openrouterApiKey) {
    config.providers.openrouter = {
      apiKey: env.openrouterApiKey,
    };
    config.defaultProvider = "openrouter";
  }

  return config;
}
