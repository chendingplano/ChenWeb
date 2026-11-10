## Context

Four document reviewers check one extracted artifact per LLM call: `metrics`,
`provisions`, `inventory_items` and `metrics_completeness` (one call per object's metric
roster). Each call is laid out as `<DOCUMENT_INPUT>` source text first, then the
reviewer's rubric, then the artifact payload (ADR 2026070201, AR2). Today the source text
is the fixed 200-line block (`buildChunkInputs`, `DefaultChunkInputSize = 200`) that
contains the artifact's first line, serialized by `RawLinesToJSON` as
`{"doc_context": …, "lines": [{"flag","line_number","page_number","line_type","content"}, …]}`.
The block was chosen to be byte-identical to the per-chunk reviewers' inputs, so the call
would reuse their cached prefix.

The artifacts themselves were extracted from different, smaller units: the extraction
chunks that the doc-processor's per-chunk batching coordinator hands to every chunk-based
extractor (`extract_metrics`, `extract_provisions`, `extract_inventory_items`, …). They are
built by `BuildChunks` (`CHUNK_SIZE`, default 2,000 bytes here, with
`CHUNK_OVERLAP_PERCENT` = 20) and saved per record as
`$ARTIFACT_DIR/<group>/<record>/<base>.chunks`, listing each chunk's overlap lines and
regular lines. The coordinator reads that file back, so the saved chunks are exactly what
the extractors read. Record 416 has 10 chunks, matching its 10 `extract_provisions` calls.

## Goals / Non-Goals

**Goals:**
- Each artifact review call carries only the extraction chunk its artifact came from.
- The passage is serialized without the per-line JSON keys.
- The four artifact reviewers behave the same way, from one shared code path.

**Non-Goals:**
- The per-chunk and per-block reviewers (`legal_compliance`, `readability`, …): they keep
  their 200-line JSON windows. Moving them, and the extractors, to the compact format is a
  separate change, because their prompts and the extractors' gold benchmarks would have to
  be re-validated.
- The matched artifacts' `source_context` (already `{line_number, content}` pairs) and the
  tools' outputs.
- The `entities` reviewer (no source window today).

## Decisions

### 1. The source passage is the saved extraction chunk

Load the record's lines (as today) and its saved chunks through a new exported
`docprocessing.LoadRecordChunks(rec, lines)` that wraps `loadChunksFromArtifactFile` and
`buildChunkArtifactBaseName` with `ARTIFACT_DIR`. When the file is missing (records chunked
before the file existed), rebuild with `BuildChunks` and the configured `CHUNK_SIZE` /
`CHUNK_OVERLAP_PERCENT`, and log a warning naming the record.

*Alternatives:* recompute chunks every time (wrong whenever `CHUNK_SIZE` changed after
extraction); a ±N-line slice or a heading-bounded section (new, unvalidated boundaries, and
not what the extractor saw); keep the 200-line block (the problem being fixed).

### 2. Choosing the chunk for an artifact

Each chunk covers a contiguous line range: its overlap lines (the previous chunk's tail),
then its regular lines. Every line is regular in exactly one chunk. For an artifact whose
spans run from line `s` (earliest start) to line `e` (latest end):

1. Prefer the chunk whose regular lines contain `s` **and** whose range contains `e`.
2. Otherwise, a chunk whose range contains `s..e` (the artifact sits in the overlap of the
   next chunk, which is the chunk that could have produced it).
3. Otherwise, the chunk whose regular lines contain `s`, with `context_truncated: true`
   (spans run past the chunk; a `metrics_completeness` roster can span chunks).
4. No chunk contains `s` (unparseable spans, filtered lines) → no passage; the call sends
   the payload alone, as today when no window resolves.

### 3. Compact line format

```text
Document: 农村生活垃圾分类处理规范 | DB33/T 2030—2018
[36 h1] 3 术语和定义
[43] 分类投放要定时、分类收集要定人、分类运输要定车、分类处理要定位的农村生活垃圾处置要求。
[119 caption] 表1 易腐垃圾及其他垃圾主要处理模式
[121 table] 121#h0: 序号 | 垃圾类型 | 处理模式 | 技术要求 | 适用范围
121#r1: 1 | 易腐垃圾 | 机器成肥 | …
```

- First line `Document: <doc_context>` (`buildDocContextLine`), omitted when empty.
- One text line per document line: `[<line_number>] <content>`, with a tag only where the
  type matters to a reviewer: `heading-N` → `hN`, `table`, `table-caption` → `caption`,
  `equation` → `eq`; any other type except `paragraph` and `list-item` keeps its raw name.
- Lines whose type ends in `image` hold only an image path and are left out.
- Table lines keep `LLMLineContent`'s numbered rows (`121#r1: …`, one per text line), so
  row citations are unchanged; newlines inside any other line are replaced by spaces, so
  every other text line starts with `[n]`.
- Dropped: `flag` (overlap lines are context like the rest), `page_number` (no artifact
  reviewer cites pages; finding locations come from line spans).

Measured on record 416: the whole document is 21,561 characters as JSON and 8,238 compact;
chunk 2 (lines 22–50) is 953 characters compact, about 460 tokens against about 8,000 for
today's window (estimated from character counts).

*Alternatives:* a compact JSON array (`[[43,"text"],…]`, still quotes and escapes every
line); TSV (tabs and type columns on every line); keep JSON and only drop keys (half the
saving).

### 4. Scheduling stays grouped, keyed by chunk

`runArtifactUnitsWindowGrouped` keeps the seed → stagger → remainder order, with units keyed
by chunk index instead of window index. Grouping costs nothing and still lets calls that
share a chunk share a prefix; it no longer drives what the call contains.

### 5. New prompt versions

`prompt-review-metrics-v7.md`, `prompt-review-metrics-missing-v2.md`,
`prompt-review-inventory-items-v4.md`, `prompt-review-provisions-v6.md` change only the
input description (Part 1, `artifact_line_spans`, `context_truncated`) and the word
"window" where it means the passage. Earlier versions stay so old runs can be reproduced.

## Reused modules

The catalogue holds nothing for this (searched `capabilities`, `search document lines
serialize llm input`, `search chunk artifact load`). Uncatalogued code reused as is:
`loadChunksFromArtifactFile`, `buildChunkArtifactBaseName`, `BuildChunks` (fallback),
`LLMLineContent`, `buildDocContextLine`. New: the compact line renderer
(`RenderLinesCompact` in `doc-processing`), registered with `create-reusable-module` because
the per-chunk reviewers and extractors are its likely next users.

## Risks / Trade-offs

- [The chunk may not hold the document's scope clause or a definition used far away] →
  the `Document:` line names the document; `get_document_metadata` and
  `get_artifact_context` remain available; the prompts say the passage is the extraction
  chunk.
- [A `metrics_completeness` roster spans several chunks] → `context_truncated: true` plus
  `get_artifact_context`, as for window-crossing artifacts today.
- [Saved chunks no longer match the line file after a re-parse] → the same risk the
  extractors already carry; missing lines are skipped by `loadChunksFromArtifactFile`.
- [Calls stop sharing a prefix with the per-chunk reviewers] → accepted; the uncached cost
  of an artifact call falls by more than the cache saved.
- [Different input changes review output] → compare runs on record 416 (provisions) and a
  metrics-heavy record before and after.

## Migration Plan

Deploy code, prompts and `doc-review.local.toml` together, then restart the server (the
review config is read once). Rollback: revert the code and the config together; the new
prompts describe the compact chunk input and must not be paired with the old code.
