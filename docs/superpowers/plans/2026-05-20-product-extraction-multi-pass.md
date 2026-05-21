# Product Extraction Multi-Pass Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single-pass product extraction flow with a multi-pass pipeline that separates mention recall, deterministic deduplication, relation enrichment, and optional metadata enrichment.

**Architecture:** Keep `kb.products` as the final output table, but introduce internal mention and candidate stages inside the processor. Use focused prompts for each stage, deterministic merge and dedup helpers in Go, and backward-compatible env loading so the processor can be adopted incrementally.

**Tech Stack:** Go, existing doc-processing pipeline, OpenAI JSON extractor, Go tests.

---

## Chunk 1: Tests And Helper Boundaries

### Task 1: Add failing tests for multi-pass helpers

**Files:**
- Create: `ChenWeb/server/api/doc-processing/extract-products_test.go`
- Modify: `ChenWeb/server/api/doc-processing/extract-products.go`
- Test: `ChenWeb/server/api/doc-processing/extract-products_test.go`

- [ ] Step 1: Write failing tests for mention normalization, candidate merge, and final dedup
- [ ] Step 2: Run `go test ./server/api/doc-processing -run 'TestProductsProcessor|TestMergeProduct'`
- [ ] Step 3: Implement minimal helper types and functions to satisfy the tests
- [ ] Step 4: Re-run the focused tests

## Chunk 2: Processor Pipeline

### Task 2: Replace single-pass extraction with mention pass + relation pass

**Files:**
- Modify: `ChenWeb/server/api/doc-processing/extract-products.go`
- Test: `ChenWeb/server/api/doc-processing/extract-products_test.go`

- [ ] Step 1: Add pass-specific prompt/model config fields and env loading
- [ ] Step 2: Wire Pass 1 mention extraction across blocks
- [ ] Step 3: Add deterministic merge/dedup between passes
- [ ] Step 4: Wire Pass 2 relation enrichment for each candidate
- [ ] Step 5: Re-run focused tests

## Chunk 3: Optional Enrichment And Regression Coverage

### Task 3: Add optional translation and categorization passes

**Files:**
- Modify: `ChenWeb/server/api/doc-processing/extract-products.go`
- Test: `ChenWeb/server/api/doc-processing/extract-products_test.go`

- [ ] Step 1: Add optional translation and categorization helpers
- [ ] Step 2: Preserve final `kb.products` row shape and artifact output
- [ ] Step 3: Add regression tests for prompt loading and end-to-end row output
- [ ] Step 4: Run `go test ./server/api/doc-processing`

Plan complete and saved to `docs/superpowers/plans/2026-05-20-product-extraction-multi-pass.md`. Ready to execute.
