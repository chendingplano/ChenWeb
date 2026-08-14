# PDF Parser JetStream Recovery Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make PDF parser JetStream startup recover safely across machine crashes and persisted stream-name changes.

**Architecture:** Keep configured stream names as preferred names, but resolve an existing stream by subject before creating one. Distinguish not-found errors from all other JetStream API failures. Cover the behavior with isolated async unit tests.

**Tech Stack:** Python 3.13, nats-py, pytest, mise.

---

## Chunk 1: Stream resolution regression coverage

**Files:**
- Modify: `ChenWeb/python/pdf-parser/tests/test_pdf_parser.py`

- [ ] Add focused async tests for existing subject-owned stream reuse, missing-stream creation, and propagation of a non-not-found JetStream error.
- [ ] Run the focused tests and confirm they fail against the current broad-exception implementation.

## Chunk 2: Robust JetStream stream setup

**Files:**
- Modify: `ChenWeb/python/pdf-parser/pdf_parser.py:648-700`

- [ ] Add narrow not-found detection and subject-based stream lookup.
- [ ] Make `_ensure_stream` reuse the discovered stream and create only when the subject is unowned.
- [ ] Run the focused tests and then the full parser test suite.

## Chunk 3: Restart configuration and documentation

**Files:**
- Modify: `ChenWeb/python/pdf-parser/mise.toml:45-53,97-105`
- Modify: `ChenWeb/python/pdf-parser/README.md:99-110`

- [ ] Keep stream defaults compatible with the existing persisted stream names while preserving environment overrides.
- [ ] Document subject-based recovery and the operational check for inspecting streams.
- [ ] Run parser tests, syntax checks, and a dry startup configuration check.

## Chunk 4: Commit and verify

- [ ] Review the diff and repository status.
- [ ] Commit with `jj commit`.
- [ ] Verify the final `jj log` is linear and contains only the intended commits.
