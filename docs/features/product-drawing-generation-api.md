# Product Drawing Generation API

ChenWeb exposes an authenticated endpoint for generating a fresh labeled 3D ventilator drawing.

## Endpoint

```http
POST /api/v1/product-drawings
Content-Type: application/json
```

The request body is optional. The default request generates the canonical ventilator exploded-view drawing:

```json
{}
```

Optional fields are `subject` (`ventilator`) and `model`. The model defaults to `IMAGE_GEN_MODEL`, or `openai-image-2.5` when that variable is unset. Only the configured model is accepted.

## Response

Successful requests return `201 Created` with `Content-Type: image/png`. The image is also saved to:

`/Users/cding/Workspace/KnowledgeStore/doc-repo/resources/product-drawings/`

The `X-Drawing-Filename` response header contains the saved filename. Each filename is unique and existing files are not overwritten.

## Configuration

- `IMAGE_GEN_BASE_URL`: OpenAI-compatible API base URL.
- `IMAGE_GEN_API_KEY`: server-side provider API key.
- `IMAGE_GEN_MODEL`: configured image model; defaults to `openai-image-2.5`.
- `PRODUCT_DRAWINGS_DIR`: optional output-directory override; defaults to the shared KnowledgeStore path.
- `PROMPTS_DIR`: optional prompt-directory override; defaults to ChenWeb's `prompts` directory.

Provider failures return `502`; invalid requests return `400`; local prompt or filesystem failures return `500`.
