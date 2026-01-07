/////////////////////////////////////////////////////////////////////
// ChenWeb/web/src/lib/config/+server.ts
// DEPRECATED - This file has been moved and restructured
//
// ⚠️  DO NOT USE THIS FILE DIRECTLY ⚠️
//
// The configuration system has been split into:
//
// 1. Server-side loading (for API endpoints, hooks, etc.):
//    Import from: $lib/config/server-config-loader.server.ts
//    Example:
//      import ServerConfigLoader from '$lib/config/server-config-loader.server';
//      const config = ServerConfigLoader.getInstance();
//
// 2. Client-side loading (for Svelte components, browser code):
//    Import from: $lib/config/config-client.ts
//    Example:
//      import { loadConfig } from '$lib/config/config-client';
//      const config = await loadConfig();
//
// 3. API Endpoint (provides config to frontend):
//    URL: /api/config
//    File: src/routes/api/config/+server.ts
//
// This restructuring fixes the browser compatibility error that occurred
// when trying to use Node.js 'fs' module in client-side code.
//
// Modified: 2026/01/03
/////////////////////////////////////////////////////////////////////

// Re-export types for backwards compatibility
export type {
  AppConfig,
  ServerConfig,
  DatabaseConfig,
  AppTableNamesConfig,
  AuthConfig,
  FullConfig
} from './server-config-loader.server';

// Note: The ServerConfigLoader class is NOT re-exported here because
// it can only be used in server-side code. Import it directly from
// server-config-loader.server.ts when needed.
