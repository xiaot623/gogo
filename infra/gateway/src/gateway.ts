import type { Provider, ProviderConfig, GatewayConfig } from "./types.js";
import { OpenRouterProvider } from "./providers/index.js";

type ProviderFactory = (config: ProviderConfig) => Provider;

// Registry of provider factories
const providerFactories: Record<string, ProviderFactory> = {
  openrouter: (config) => new OpenRouterProvider(config),
};

/**
 * Register a custom provider factory
 */
export function registerProviderFactory(name: string, factory: ProviderFactory): void {
  providerFactories[name] = factory;
}

/**
 * LLM Gateway - Unified interface to multiple LLM providers
 */
export class Gateway {
  private providers: Map<string, Provider> = new Map();
  private defaultProvider?: string;

  constructor(config: GatewayConfig) {
    this.defaultProvider = config.defaultProvider;

    for (const [name, providerConfig] of Object.entries(config.providers)) {
      const factory = providerFactories[name];
      if (!factory) {
        throw new Error(`Unknown provider: ${name}. Available: ${Object.keys(providerFactories).join(", ")}`);
      }
      this.providers.set(name, factory(providerConfig));
    }

    // Set default provider if not specified
    if (!this.defaultProvider && this.providers.size > 0) {
      this.defaultProvider = this.providers.keys().next().value;
    }
  }

  /**
   * Get a provider by name
   */
  getProvider(name?: string): Provider {
    const providerName = name ?? this.defaultProvider;
    if (!providerName) {
      throw new Error("No provider specified and no default provider configured");
    }

    const provider = this.providers.get(providerName);
    if (!provider) {
      throw new Error(`Provider not found: ${providerName}. Available: ${Array.from(this.providers.keys()).join(", ")}`);
    }

    return provider;
  }

  /**
   * List all registered providers
   */
  listProviders(): string[] {
    return Array.from(this.providers.keys());
  }

  /**
   * Get the default provider name
   */
  getDefaultProvider(): string | undefined {
    return this.defaultProvider;
  }
}
