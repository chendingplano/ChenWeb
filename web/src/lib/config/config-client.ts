/////////////////////////////////////////////////////////////////////
// ChenWeb/web/src/lib/config/config-client.ts
// Client-side configuration loader that fetches config from the API
//
// This module provides a browser-safe way to access application config
// by fetching it from the /api/config endpoint.
//
// Usage in Svelte components or client code:
// import { loadConfig } from '$lib/config/config-client';
//
// const config = await loadConfig();
// console.log('App Name:', config.app_name);
//
// Created: 2026/01/03
/////////////////////////////////////////////////////////////////////

import type { FullConfig } from './server-config-loader.server';

let cachedConfig: FullConfig | null = null;

/**
 * Loads the application configuration from the API endpoint.
 * Results are cached after the first successful load.
 *
 * @returns Promise that resolves to the full configuration object
 * @throws Error if the configuration cannot be loaded
 */
export async function loadConfig(): Promise<FullConfig> {
  if (cachedConfig) {
    return cachedConfig;
  }

  try {
    const response = await fetch('/api/config');

    if (!response.ok) {
      throw new Error(`Failed to load config: ${response.statusText}`);
    }

    cachedConfig = await response.json();
    return cachedConfig!;
  } catch (error: any) {
    console.error('[CONFIG CLIENT] Failed to load configuration:', error);
    throw new Error(`Configuration load failed: ${error.message}`);
  }
}

/**
 * Clears the cached configuration, forcing a reload on the next call to loadConfig()
 */
export function clearConfigCache(): void {
  cachedConfig = null;
}
