# China Mechanical Product Names Import Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an admin importer that appends the supplied Chinese mechanical product catalog to `kb.product_names` under source `china-mechanical`.

**Architecture:** Add six text columns with a ChenWeb goose migration. Create a narrowly scoped backend import handler that parses uploaded CSV, translates product names through the configured translation model, and inserts rows idempotently without modifying existing records. Add an embedded Svelte admin view, client API, and Resources navigation entry.

**Tech Stack:** Go, Echo, PostgreSQL/goose, Svelte 5, TypeScript, configured LLM client.

---

## Chunk 1: Storage and importer API

Files:
- Create `project_migrations/20260925000001_add_china_mechanical_product_name_columns.sql` for the six additive text columns.
- Create `server/api/productnameimporthandler/handler.go` to validate/parse CSV, translate names, and insert only absent records.
- Modify `server/api/routes.go` to register admin-only preview/import routes.

## Chunk 2: Admin page

Files:
- Create `web/src/lib/components/home3/mechanical-product-names-import-view.svelte` with file selection, preview, import, and outcome display.
- Create `web/src/lib/components/home3/mechanical-product-names-import-client.ts` for typed multipart API calls.
- Modify `web/src/lib/components/home3/nav-rail.svelte` and `content-panel.svelte` to add the Resources child entry and mount the view.

## Chunk 3: Review and verification

- Check formatting and run focused Go and frontend checks as appropriate.
- Review the resulting schema, access control, CSV mapping, and additive conflict behavior.
- Update this plan's checkboxes as work completes.
