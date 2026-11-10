## Why

The artifact reviewers (metrics, provisions, inventory items, metrics completeness) review
one artifact per LLM call, yet every call carries the fixed 200-line block of the document
that contains the artifact, serialized as one JSON object per line. ADR 2026070201 (AR2)
chose that block so these calls would share a cached prefix with the per-chunk reviewers.
In review_provisions run 2874 (record 416, 169 lines) every one of 79 calls carried the
whole 12-page document: about 8,000 of each call's 12,000 input tokens, close to half of
them JSON keys (`flag`, `line_number`, `page_number`, `line_type`) repeated on every line.
A cache hit is not a reason to send text the reviewer does not need: unrelated pages dilute
the model's attention, and every uncached call pays for them in full.

## What Changes

- Each artifact review call carries the **extraction chunk** the artifact came from: the
  chunk (about 2,000 bytes, with overlap lines) that the extractors read, taken from the
  record's saved `.chunks` file. It replaces the 200-line block. For record 416, provision
  416_prv_1 gets lines 22–50 (the terms-and-definitions clause it was taken from) instead
  of lines 1–169.
- The passage is serialized in a **compact line format**, one text line per document line
  (`[43] text`, with a short type tag only for headings, tables, captions and equations),
  instead of the per-line JSON wrapper. Lines that hold only an image path are left out.
- Units are still grouped by their source passage and run seed → stagger → remainder
  (AR3), now keyed by chunk; the per-chunk reviewers' 200-line windows are unchanged.
- New prompt versions for the four reviewers describe the new input:
  `prompt-review-metrics-v7.md`, `prompt-review-metrics-missing-v2.md`,
  `prompt-review-inventory-items-v4.md`, `prompt-review-provisions-v6.md`, and
  `doc-review.local.toml` points at them.
- ADR 2026070201 gains an amendment recording that AR2's source window is replaced by the
  extraction chunk and that cache reuse no longer decides what the call contains.
- Removed: the 200-line window code path for artifact reviewers
  (`loadArtifactReviewWindows`, `windowIndexForSpans`, `spansTruncatedByWindow`).

## Capabilities

### New Capabilities
- `artifact-review-input`: what source text an artifact review call carries (which
  extraction chunk, how it is chosen, when it is flagged as truncated) and how that text is
  serialized for the model.

### Modified Capabilities
<!-- None: no existing spec in openspec/specs covers document review input. -->

## Impact

- Code (`server/api/doc-reviews/`): `review-artifact-window.go` (replaced by chunk loading,
  selection and compact rendering), `review-metrics.go`, `review-provisions.go`,
  `review-inventory-items.go`, `review-metrics-completeness.go`, and their tests.
- Code (`server/api/doc-processing/`): `review_exports.go` gains an exported loader for a
  record's saved extraction chunks (wrapping `loadChunksFromArtifactFile`).
- Prompts: four new versions (above); earlier versions stay for reproducing old runs.
- Config: `doc-review.local.toml` prompt names for the four reviewers. Restart the server
  after deploying (the review config is read once).
- Docs: ADR `KnowledgeStore/doc-repo/adrs/202607/2026070201-adr-document-review-changes.md`
  (amendment).
- Cost: per call, the document part drops from about 8,000 to about 500 tokens for record
  416 (estimated from character counts); calls that share a chunk still share a prefix.
- Not changed: the per-chunk and per-block reviewers, the extractors' own input format,
  the matched artifacts' `source_context`, the tools.
- Reusable modules: searched the catalogue (`capabilities`; `search document lines
  serialize llm input`; `search chunk artifact load`) and nothing fit. Reuses uncatalogued
  code: `loadChunksFromArtifactFile` / `buildChunkArtifactBaseName` (saved chunks),
  `LLMLineContent` (table rows rendered as numbered rows), `buildDocContextLine` (document
  title and number). The compact line renderer is new and is registered for reuse.
- IDs: the change allocates no IDs.
