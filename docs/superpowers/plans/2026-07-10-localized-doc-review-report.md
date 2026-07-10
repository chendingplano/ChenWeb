# Localized Document Review Report Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fully localize Chinese Typst document-review reports without duplicating their layout.

**Architecture:** Add a language-normalized report lexicon in the report generator and pass its static labels, Typst language, and font to the existing template. The template renders its labels from that dictionary; Go uses the lexicon for dynamically emitted headings and summaries.

**Tech Stack:** Go 1.25, Typst, existing Go unit tests.

---

### Task 1: Localized source generation

**Files:**
- Modify: `server/api/doc-reviews/typst_report.go`
- Test: `server/api/doc-reviews/typst_report_test.go`

- [ ] Write a failing test that builds a Chinese report source and requires Chinese template labels, package/aspect labels, and generated assessment text.
- [ ] Run the focused test and verify it fails because labels remain English.
- [ ] Add a small `reportLexicon`, select it from the report language, and use it for Go-generated text and template arguments.
- [ ] Run the focused test and verify it passes.

### Task 2: Parameterized Typst chrome

**Files:**
- Modify: `docs/doc-templates/template-document-report.typ`
- Test: `server/api/doc-reviews/typst_report_test.go`

- [ ] Replace hard-coded template chrome with values from `labels`; add language and font parameters.
- [ ] Compile the generated Chinese Typst source to PDF in the existing test suite.
- [ ] Run `go test ./server/api/doc-reviews -count=1`.
- [ ] Commit the implementation and tests with jj.
