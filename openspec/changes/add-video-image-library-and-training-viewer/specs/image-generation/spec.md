## ADDED Requirements

### Requirement: AI cover generation
The system SHALL provide an authenticated endpoint that generates an image from a text prompt using an OpenAI-compatible text-to-image API (`POST {base}/v1/images/generations`), configured via environment (`IMAGE_GEN_BASE_URL`, `IMAGE_GEN_API_KEY`, `IMAGE_GEN_MODEL`). On success the generated image SHALL be persisted into the image library (`kb.images`, origin = generated, with the prompt recorded) and returned to the caller.

#### Scenario: Successful generation
- **WHEN** an authenticated user requests generation with a prompt (e.g. derived from a video's name/description) and the provider is configured
- **THEN** the system calls the provider, saves the returned image into `kb.images`, and returns the new image's metadata so the caller can select it as a cover

#### Scenario: Each click yields a new image
- **WHEN** the user clicks Auto-Generate repeatedly
- **THEN** each click produces and saves a new distinct image (no overwrite of a prior one)

### Requirement: Graceful degradation when unconfigured
When the image-generation provider is not configured or the provider call fails, the endpoint SHALL return a clear error WITHOUT crashing, and the rest of the flow (Pick an Image, manual upload, video upload) SHALL remain usable.

#### Scenario: Provider not configured
- **WHEN** `IMAGE_GEN_BASE_URL`/`IMAGE_GEN_API_KEY`/`IMAGE_GEN_MODEL` are missing
- **THEN** Auto-Generate returns a clear "image generation not configured" error and no partial `kb.images` row is left behind

#### Scenario: Provider error
- **WHEN** the provider returns an error or times out
- **THEN** the endpoint surfaces a clear error and leaves no partial image row or file
