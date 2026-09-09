# Product Drawing Generation API Design

## Goal

Add a ChenWeb API that generates a fresh, labeled 3D ventilator drawing with an OpenAI image-generation model on every request and saves the PNG to the shared KnowledgeStore product-drawings directory.

## Context

ChenWeb is a Go service with versioned `/api/v1` routes and authenticated request handling. The canonical image prompt must live in a versioned file under `prompts/`. Generated product drawings are shared workspace assets stored at:

`/Users/cding/Workspace/KnowledgeStore/doc-repo/resources/product-drawings/`

## Architecture

Create a focused `server/api/productdrawings` package. It owns the request/response types, canonical prompt loading, provider abstraction, filename generation, and handler behavior. The OpenAI implementation is injected behind a small provider interface so unit tests do not call the live service.

Register the handler in `server/api/routes.go` under the existing authenticated `/api/v1` route group. The handler generates an image synchronously, writes it to the shared directory using an exclusive unique filename, and streams the same PNG to the client.

The OpenAI API key remains server-side and is read through ChenWeb's existing configuration/environment conventions. The default model is the configured OpenAI Image 2.5 model; the request may select another allowlisted model only if ChenWeb configuration supports it.

## Endpoint contract

### Request

```http
POST /api/v1/product-drawings
Content-Type: application/json
Accept: image/png
```

```json
{
  "subject": "ventilator",
  "model": "openai-image-2.5"
}
```

Both fields are optional. `subject` defaults to `ventilator`. The canonical ventilator prompt is used for that subject. The model defaults to the configured OpenAI Image 2.5 model.

The first version does not accept arbitrary caller-supplied prompts. This preserves the requested “same drawing” behavior and prevents unreviewed prompt content from becoming an image-generation surface.

### Success response

- Status: `201 Created`
- `Content-Type: image/png`
- `Content-Disposition: attachment; filename="ventilator-exploded-view-<timestamp>-<id>.png"`
- `X-Drawing-Filename`: the exact saved filename
- Body: generated PNG bytes

The filename includes a UTC timestamp and random identifier. Existing files are never overwritten.

### Error responses

Errors use a small JSON envelope such as `{ "error": "..." }`:

- `400 Bad Request`: malformed JSON, unsupported subject, or unsupported model.
- `401 Unauthorized`: handled by the existing route middleware.
- `502 Bad Gateway`: OpenAI/provider failure or invalid provider image payload.
- `500 Internal Server Error`: prompt loading or local file-system failure.

No response includes credentials, provider request headers, or internal secrets.

## Canonical prompt

Store the approved prompt in:

`prompts/prompt-ventilator-exploded-view-v1.md`

It will specify a medical-device technical illustration, a three-quarter exploded view, mechanically plausible ventilator components, numbered callouts, and the visible parts legend. The API reads this file rather than embedding the prompt in Go source.

## Components and responsibilities

- `server/api/productdrawings/models.go`: request, provider request, and handler metadata types.
- `server/api/productdrawings/prompt.go`: canonical prompt loading and subject-to-prompt selection.
- `server/api/productdrawings/provider.go`: image-provider interface and OpenAI implementation.
- `server/api/productdrawings/handler.go`: HTTP validation, generation, persistence, headers, and response.
- `server/api/productdrawings/handler_test.go`: HTTP behavior and failure cases using a fake provider.
- `server/api/productdrawings/provider_test.go`: provider request construction and response decoding.
- `prompts/prompt-ventilator-exploded-view-v1.md`: canonical prompt text.
- `server/api/routes.go`: route registration.

## Data flow

1. Authenticated client sends `POST /api/v1/product-drawings`.
2. Handler decodes the optional request body and applies default subject/model values.
3. Prompt loader selects the versioned ventilator prompt.
4. Provider sends the prompt and selected model to OpenAI.
5. Handler validates that the provider returned non-empty PNG data.
6. Handler creates the shared directory if needed and writes a new exclusive file.
7. Handler returns the PNG bytes and saved filename header.

## Testing

Tests will be written before implementation and will cover:

- default ventilator subject and default model;
- successful `201` PNG response and required headers;
- saved output in the shared directory with a unique filename;
- malformed JSON and unsupported subject/model rejection;
- provider failure mapping to `502`;
- empty/invalid provider image data mapping to `502`;
- prompt loading from the versioned prompt file;
- OpenAI request payload construction without exposing the API key.

Verification will include focused package tests, `go build ./...` from `ChenWeb/server`, and relevant route tests. Any pre-existing unrelated failures will be recorded separately.

## Non-goals

- Asynchronous job tracking or polling.
- Database persistence of drawings or generation jobs.
- Arbitrary caller-supplied prompts.
- Returning base64 JSON by default.
- Editing or replacing an existing drawing.
- Adding frontend UI in this change.
