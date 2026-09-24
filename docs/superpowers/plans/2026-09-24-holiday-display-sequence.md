# Holiday Display Sequence Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add unique per-country holiday display sequencing with append-on-create and editable ordering.

**Architecture:** Persist `display_seqno` on `public.holiday_info`, expose it through the existing calendar handler API, and keep reordering logic transactional in the SQL store layer. The existing Svelte admin table and modal will surface and submit the sequence number.

**Tech Stack:** PostgreSQL goose-style SQL migrations, Go `database/sql`, Echo handlers, Svelte 5, TypeScript.

---

## Chunk 1: Database and backend behavior

### Task 1: Add the schema migration

**Files:**
- Create: `project_migrations/<timestamp>_add_holiday_display_seqno.sql`

- [ ] Add the non-null column with a temporary safe default or staged migration.
- [ ] Backfill each country’s existing rows with `ROW_NUMBER() OVER (PARTITION BY country ORDER BY id)`.
- [ ] Add `CHECK (display_seqno > 0)` and `UNIQUE (country, display_seqno)`.
- [ ] Add the migration down section following existing project conventions.

### Task 2: Write failing store tests

**Files:**
- Create or modify: `server/api/calendarhandler/store_test.go`

- [ ] Add tests proving a new holiday gets the country maximum plus one.
- [ ] Add tests proving list order follows `display_seqno`.
- [ ] Add tests proving an in-country move shifts intervening rows and leaves unique contiguous values.
- [ ] Add a test proving a country move removes the old position and inserts into the new country.
- [ ] Run the focused tests and confirm they fail because the field and behavior do not yet exist.

### Task 3: Implement backend sequencing

**Files:**
- Modify: `server/api/calendarhandler/store.go`
- Modify: `server/api/calendarhandler/handler.go`

- [ ] Add `DisplaySeqno` to `HolidayInfo` and include it in scanning/returning columns.
- [ ] Make list queries order by country and display sequence.
- [ ] Implement transactional append numbering on create.
- [ ] Implement transactional remove-and-insert shifting on update, including country changes.
- [ ] Validate positive sequence values and map invalid values to HTTP 400.
- [ ] Preserve existing duplicate-name conflict handling.
- [ ] Run focused Go tests and confirm they pass.

## Chunk 2: Frontend and contract documentation

### Task 4: Write failing frontend/type checks

**Files:**
- Modify: `web/src/lib/components/home3/calendar-admin-client.ts`
- Modify: `web/src/lib/components/home3/calendar-admin-view.svelte`

- [ ] Update the client type and input type to include `display_seqno`.
- [ ] Add the sequence column to the definitions table.
- [ ] Add a positive-number input to the edit/new form and submit it with create/update requests.
- [ ] Keep newly created holiday definitions visible in sequence order after reload.
- [ ] Run the frontend type checker before implementation changes to capture the expected type failures.

### Task 5: Update the canonical requirements and devdoc

**Files:**
- Modify: `openspec/specs/holiday-calendar-admin/spec.md`
- Modify: `../KnowledgeStore/doc-repo/devdocs/202609/2026092401-devdoc-holiday-calendar-admin.md`

- [ ] Document that holiday definitions have unique positive per-country display sequence numbers.
- [ ] Document append-on-create, transactional repositioning, and ordered listing.
- [ ] Update schema/API details and verification status without changing unrelated feature history.

### Task 6: Verify the complete change

- [ ] Run `go test ./server/api/calendarhandler` (or the project-equivalent focused package command).
- [ ] Run `go build ./server/...` or the project’s documented server build command.
- [ ] Run the frontend `svelte-check` command from `web`.
- [ ] Inspect the final diff and confirm no unrelated pre-existing worktree changes were modified.

