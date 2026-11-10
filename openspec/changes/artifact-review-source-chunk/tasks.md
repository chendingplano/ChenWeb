## 1. Chunk loading and compact rendering (doc-processing)

- [x] 1.1 Add `RenderLinesCompact(lines []MarkedLine, docCtx string) string` (compact line
      format, design §3) with unit tests: paragraph, heading tag, table rows, caption,
      equation, image lines left out, newline replaced, empty doc context
- [x] 1.2 Export `LoadRecordChunks(rec DocMetadataInputRecord, lines []Line) (chunks []Chunk, rebuilt bool, err error)`
      in `review_exports.go`: saved `.chunks` file under `ARTIFACT_DIR`, fallback to
      `BuildChunks` with `CHUNK_SIZE` / `CHUNK_OVERLAP_PERCENT`, reporting which was used

## 2. Artifact review input (doc-reviews)

- [x] 2.1 Replace `loadArtifactReviewWindows` with a loader that returns the record's
      source chunks (line range, regular range, compact text) and logs a warning on the
      rebuild fallback
- [x] 2.2 Replace `windowIndexForSpans` / `spansTruncatedByWindow` with the chunk choice of
      design §2 (returns chunk index and truncated flag), with unit tests for the four
      cases in the spec
- [x] 2.3 Switch `provisions`, `metrics`, `inventory_items` and `metrics_completeness` to
      the chunk input; key the grouped scheduler's units by chunk index (`runArtifactUnitsChunkGrouped`)
- [x] 2.4 Remove the window helpers that are no longer used and update comments that
      describe the 200-line window for artifact reviewers
- [x] 2.5 Update the reviewers' existing tests; `go test ./api/doc-reviews/ ./api/doc-processing/`
      passes apart from known unrelated failures

## 3. Prompts and configuration

- [x] 3.1 Write `prompt-review-provisions-v6.md`, `prompt-review-metrics-v7.md`,
      `prompt-review-metrics-missing-v2.md`, `prompt-review-inventory-items-v4.md`: input
      description (Part 1, `artifact_line_spans`, `context_truncated`) for the compact
      extraction chunk
- [x] 3.2 Point the four reviewers in `doc-review.local.toml` at the new prompts

## 4. Documentation

- [x] 4.1 Add the AR2/AR3 amendment to ADR 2026070201 (KnowledgeStore)
- [x] 4.2 Register `RenderLinesCompact` with the `create-reusable-module` skill and run the
      catalogue check (module 20261110-01, capability `llm-input`)

## 5. Verification

- [x] 5.1 Render record 416's provision inputs offline and confirm 416_prv_1 maps to chunk 2
      (lines 22–50) and the passage matches the compact format
- [ ] 5.2 After a server restart, run `review_provisions` on record 416 and compare input
      tokens and findings with run 2874
