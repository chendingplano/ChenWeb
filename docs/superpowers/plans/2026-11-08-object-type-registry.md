# Object type registry implementation plan

**Goal:** Accept new object types as proposals, retain useful failure diagnostics, and let administrators review and approve types.

**Architecture:** PostgreSQL `kb.object_types` owns canonical type names and approval state. Existing compact codes remain compatibility aliases. Extraction accepts nonempty type strings; persistence registers new names atomically without changing existing approval. Object matching only gains type-based confidence for approved names. Human approval changes the existing registry row.

**Tech stack:** Go, PostgreSQL/goose, Echo, Svelte 5, Paraglide.

The user approved this design in the conversation and requested implementation, including a human review page. Work proceeds in the existing clean repositories without new branches; changes are committed separately through jj.

- [x] Add regression tests for unknown compact types, canonical aliases, concurrent proposal registration, approval, and matching semantics.
- [x] Add a reversible project migration: seed ten approved canonical types; preserve preexisting nonstandard names as proposed; register future object writes.
- [x] Implement registry store and integrate extraction, object persistence, and matching. Preserve original model type plus source provenance.
- [x] Log each failed structured-output attempt with model, prompt, contract, attempt, record/run and raw response. Keep primary/fallback errors independently inspectable.
- [x] Add authenticated administrative list/approve endpoints with authorization and handler tests.
- [x] Add an English/Chinese review page under System Admin > Database, preserving page state across navigation.
- [x] Verify migration up/down/up in an isolated local database, focused Go tests and frontend checks; review integrated changes and update durable documentation.
- [x] Commit shared, application and documentation changes separately through jj and inspect linear history.

## Validation

Run the focused `doc-processing`, `kbhandler`, and shared `llm` packages, then the complete shared Go suite and workspace dependent builds as required. Run `bun run check` and UI state/client tests. Use local PostgreSQL only; production deployment and record reruns are separate from this implementation.

## Results and baseline limitations

- Full shared Go suite and shared LLM vet pass. ChenWeb server and tax Go builds pass; `go work sync` completed without module changes.
- Full frontend production build passes with `NODE_OPTIONS=--max-old-space-size=16384`, matching the workspace's existing large-build setting. Default and 8 GB heaps were insufficient for the existing application. Focused registry/diagnostic/candidate tests also pass under the Go race detector.
- `doc-processing` suite passes with the unrelated preexisting `TestKBMetricsReadersExcludeResolvedRows` skipped: its guard flags `kbhandler/question_generator_worker.go` for missing `ref_mention_id IS NULL`.
- Focused review API tests, extraction/registry/diagnostic tests and three frontend client tests pass. Isolated PostgreSQL Up/Down/Up tests include 16 concurrent mixed SQL/Go discoveries, alias handling, raw provenance updates, and approval idempotence.
- Full `kbhandler` run exposes existing input-folder deletion guard failures and metric-score fixture/hash parity failures. Those files are outside this change.
- Full `bun run check` has three existing WPI TypeScript errors: two for `loadedPage` declaration order in `shared-ui/wpi-editor.svelte`, one JsonObject fixture error in `canvas/wpi-component-tree.test.ts`. The new page has no Svelte errors/warnings; standalone i18n passes. Existing InputElement CSS line-clamp warning remains.
- Mocked browser checks exercise approval, provenance, filtering, Chinese text, narrow-panel overflow, and actual menu leave/return restoration of search, selected row and both scroll positions.
- Independent review found and resolved proposed-type SQL ranking and stale raw provenance during type updates; follow-up review found no blockers.

## Knowledge changes

The broad object type vocabulary is now a reviewed database registry; unknown names no longer invalidate provision extraction. Updated the object-centric ADR, added `KnowledgeStore/table-schemas/table-kb-object-types.md` and devdoc `2026110804-devdoc-object-type-registry.md`. Historical plans and old prompts remain historical records; the old fixed code map remains only as a compact-wire compatibility adapter. Synonym merge/rejection/retirement workflows and production fallback quota remediation are outside this change.
