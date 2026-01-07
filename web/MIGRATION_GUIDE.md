# Configuration System Migration Guide

## Problem

The original configuration loader in `src/lib/config/+server.ts` was trying to use Node.js's `fs` module, which caused this error:

```
Error: Module "fs" has been externalized for browser compatibility.
Cannot access "fs.readFileSync" in client code.
```

This happened because files in `src/lib/` can be imported by both client and server code, and Vite tries to make them browser-compatible.

## Solution

The configuration system has been restructured into three parts:

1. **Server-only loader** - `src/lib/config/server-config-loader.server.ts`
   - Uses Node.js `fs` module to read `config.toml`
   - The `.server.ts` suffix ensures it's NEVER bundled for the browser

2. **Client-side loader** - `src/lib/config/config-client.ts`
   - Fetches config from the API endpoint
   - Safe to use in Svelte components and browser code

3. **API endpoint** - `src/routes/api/config/+server.ts`
   - Bridges server and client
   - Accessible at `/api/config`

## How to Update Your Code

### If you were importing from the old location:

**Before:**
```typescript
import ServerConfigLoader from '$lib/config/+server';
```

**After (Server-side code):**
```typescript
import ServerConfigLoader from '$lib/config/server-config-loader.server';
```

**After (Client-side code):**
```typescript
import { loadConfig } from '$lib/config/config-client';

const config = await loadConfig();
```

### Usage Examples

#### In API Routes (`src/routes/api/*/+server.ts`)

```typescript
import { json } from '@sveltejs/kit';
import ServerConfigLoader from '$lib/config/server-config-loader.server';

export async function GET() {
  const config = ServerConfigLoader.getInstance();
  const dbConfig = config.getDatabaseConfig();

  return json({
    host: dbConfig.pg_host,
    port: dbConfig.pg_port
  });
}
```

#### In Svelte Components (`*.svelte`)

```svelte
<script lang="ts">
  import { loadConfig } from '$lib/config/config-client';
  import { onMount } from 'svelte';

  let appName = '';

  onMount(async () => {
    const config = await loadConfig();
    appName = config.app_name;
  });
</script>

<h1>Welcome to {appName}</h1>
```

#### In Server Hooks (`src/hooks.server.ts`)

```typescript
import type { Handle } from '@sveltejs/kit';
import ServerConfigLoader from '$lib/config/server-config-loader.server';

export const handle: Handle = async ({ event, resolve }) => {
  const config = ServerConfigLoader.getInstance();

  // Add config to locals for use in load functions
  event.locals.config = config.getAll();

  return resolve(event);
};
```

## Additional Changes

### `utils.ts` → `utils.server.ts`

The `checkConfigFileExists` function has been moved from `src/lib/utils.ts` to `src/lib/utils.server.ts` for the same reason.

**Before:**
```typescript
import { checkConfigFileExists } from '$lib/utils';
```

**After (Server-side only):**
```typescript
import { checkConfigFileExists } from '$lib/utils.server';
```

## Testing

1. Start the dev server:
   ```bash
   npm run dev
   ```

2. Access the config API:
   ```bash
   curl http://localhost:5173/api/config
   ```

3. You should see your config.toml contents as JSON

## Files Changed

- ✅ Created: `src/lib/config/server-config-loader.server.ts`
- ✅ Created: `src/lib/config/config-client.ts`
- ✅ Created: `src/routes/api/config/+server.ts`
- ✅ Created: `src/lib/utils.server.ts`
- ✅ Updated: `src/lib/config/+server.ts` (now just exports types)
- ✅ Updated: `src/lib/utils.ts` (removed fs import)

## Why `.server.ts`?

The `.server.ts` suffix is a SvelteKit convention that tells the bundler:
- ✅ Include this file ONLY in server builds
- ✅ Allow Node.js-specific imports (fs, path, etc.)
- ❌ Never bundle this for the browser
- ❌ Throw an error if client code tries to import it

This ensures type safety and prevents accidental use of server-only code in the browser.
