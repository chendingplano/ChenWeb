# 3D Product Drawing Review Page Design

## Goal

Add a Development dashboard page at `System Admin → Resources → Generate 3D Product Drawings` that lets an authenticated user generate a fresh ventilator drawing, review its prompt and image, then keep or ignore it.

## User flow

1. User opens the new menu item in the Development dashboard.
2. User clicks **Generate drawing**.
3. ChenWeb calls the image-generation provider and creates a pending temporary artifact.
4. The page displays the generated image, prompt, model, and pending status.
5. **Keep drawing** moves the artifact into `KnowledgeStore/doc-repo/resources/product-drawings/` under a unique filename.
6. **Ignore** permanently removes the pending artifact.
7. If the user leaves the page while an artifact is pending, the client attempts to discard it; the server also expires stale pending artifacts when they are accessed.

## Navigation and UI

Add `sysadmin-resources-product-drawings` beneath the existing `sysadmin-resources` item in `web/src/lib/components/home3/nav-rail.svelte`. Render `ProductDrawingsView.svelte` from the matching branch in `content-panel.svelte`.

The view uses Svelte 5 runes and the existing `darkMode` prop. Its visual direction is a technical workbench: deep graphite shell, paper-white drawing board, cyan/amber status accents, a compact metadata rail, and restrained reveal motion. It contains:

- title, explanatory copy, and generation button;
- idle, loading, error, pending, kept, and ignored states;
- large image preview with accessible alt text;
- collapsible prompt panel showing the exact prompt used;
- model and generated-file metadata;
- mutually clear **Keep drawing** and **Ignore** actions while pending;
- success confirmation and a new-generation action after keep/ignore.

## Backend API

Keep the existing endpoint available for compatibility, but add the pending-artifact flow:

### Generate

```http
POST /api/v1/product-drawings/generate
```

Returns JSON:

```json
{
  "token": "opaque-token",
  "image_url": "/api/v1/product-drawings/pending/opaque-token/content",
  "prompt": "...",
  "model": "openai-image-2.5",
  "expires_at": "..."
}
```

The generated PNG is stored outside the permanent product-drawings directory in a server-controlled temporary pending directory. The token is cryptographically random and maps to server-side metadata; clients never supply filesystem paths.

### Preview

```http
GET /api/v1/product-drawings/pending/:token/content
```

Returns the pending PNG with `image/png`. It requires authentication and validates the opaque token before opening the file.

### Keep

```http
POST /api/v1/product-drawings/pending/:token/keep
```

Moves the pending file into the shared `resources/product-drawings` directory using an exclusive unique filename and returns JSON metadata including the filename and path relative to the shared resource directory. The operation is idempotent for an already-kept token.

### Ignore

```http
DELETE /api/v1/product-drawings/pending/:token
```

Deletes the pending artifact. Repeating the operation is safe and returns a successful discarded result for an already-discarded token.

## Client service

Add a typed service module under `web/src/lib/services/` that uses the existing fetch/auth conventions. It exposes `generateProductDrawing`, `getPendingProductDrawingContentURL`, `keepProductDrawing`, and `ignoreProductDrawing`. It parses structured error responses and does not expose server filesystem paths to image URLs.

## Error handling and security

- Authentication remains provided by the existing `/api/v1` middleware.
- Invalid or expired tokens return `404` without revealing whether another token exists.
- Provider failures return `502`; malformed requests return `400`; local storage failures return `500`.
- Pending files have a bounded lifetime and are removed on keep/ignore and stale-token access.
- The prompt is server-owned and remains the versioned `prompts/prompt-ventilator-exploded-view-v1.md`.
- No API key, absolute filesystem path, or provider diagnostic is returned to the browser.

## Testing

Backend tests cover generation into pending storage, preview access, keep move, ignore deletion, expiration, invalid tokens, provider failure, and no permanent artifact before keep. Frontend tests cover the page state transitions, button behavior, prompt display, and cleanup on unmount/navigation.

Run focused Go tests and frontend checks, then `cd server && go build ./...`, `go vet` for touched packages, and the relevant web type/test commands.

## Non-goals

- Arbitrary user-authored prompts.
- A product-drawing database/catalog.
- Batch generation.
- Editing an image after generation.
- Changes to unrelated Resources pages.
