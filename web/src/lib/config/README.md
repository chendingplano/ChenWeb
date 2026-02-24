# Configuration System

This directory contains the configuration loading system for ChenWeb.

## Files

- **server-config-loader.server.ts** - Server-only config loader (uses Node.js `fs` module)
- **config-client.ts** - Client-side config loader (fetches from API)
- **+server.ts** - DEPRECATED, kept for type exports only

## Usage

### In Server-Side Code (API endpoints, hooks, etc.)

```typescript
import ServerConfigLoader from '$lib/config/server-config-loader.server';

// In a +server.ts file or hooks.server.ts
export async function GET() {
  const configLoader = ServerConfigLoader.getInstance();
  const dbConfig = configLoader.getDatabaseConfig();

  console.log('DB Host:', dbConfig.pg_host);

  return json({ success: true });
}
```

### In Client-Side Code (Svelte components, browser)

```typescript
import { loadConfig } from '$lib/config/config-client';

// In a +page.svelte or component
<script lang="ts">
  import { onMount } from 'svelte';

  let appName = '';

  onMount(async () => {
    const config = await loadConfig();
    appName = config.app_name;
  });
</script>

<h1>{appName}</h1>
```
Note: the function 
### Using the API Endpoint Directly

```bash
curl http://localhost:8080/api/config
```

## Why This Structure?

The original `+server.ts` file in `src/lib/config/` tried to use Node.js `fs` module, but files in `src/lib/` can be imported by both client and server code. Vite tried to bundle it for the browser, causing the error:

```
Error: Module "fs" has been externalized for browser compatibility
```

The solution:
1. **server-config-loader.server.ts** - The `.server.ts` suffix tells SvelteKit to ONLY include this in server builds
2. **config-client.ts** - Safe for browser, fetches config from the API
3. **/api/config/+server.ts** - API endpoint that bridges server and client

## Type Safety

All configuration types are exported from `server-config-loader.server.ts` and can be imported in any file:

```typescript
import type { FullConfig, DatabaseConfig } from '$lib/config/server-config-loader.server';
```
