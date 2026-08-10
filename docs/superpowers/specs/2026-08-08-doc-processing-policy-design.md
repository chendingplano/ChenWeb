# Knowledge-Store-Scoped Doc Processing Policies

> **Superseded (2026-08-10):** `kb.pipeline_policies` — the "versioned
> activation policy" this design's bootstrap tool wrote to — is retired by
> ADR `2026081001` (`KnowledgeStore/doc-repo/adrs/202608/
> 2026081001-adr-pipeline-policy-versioning-and-dependency-graph.md`). The
> `doc-processing-policy-seed` tool described below still exists and is
> still config-driven from `[doc-processing-policy-*]` sections, but it now
> authors a new `kb.pipelines` version per policy (superseding the prior
> version for that name) and upserts `kb.pipeline_bindings` rows by name,
> instead of minting/activating a `kb.pipeline_policies` row. Everywhere
> below that mentions `kb.pipeline_policies`, policy activation, or a
> "versioned activation policy" reflects the pre-ADR model and should be
> read historically.

**Status:** Draft, pending user approval (2026-08-08)
**Governing ADR:** `KnowledgeStore/doc-repo/adrs/202607/2026072901-adr-ontology-platform-and-adaptive-pipeline.md` — DR5 (§3.6), DR6 (§3.7), DR7 (§3.8), §16.1 "Outstanding Implementation Work — P1"
**Scope:** A minimal, config-authored instantiation of the existing DR6 named-pipeline + knowledge-store-binding mechanism. No new schema, no new resolution logic, no frontend.

## 1. Problem

Today, which doc processors run on every document is one flat, global list: `[doc-processing].required_processors` in `config.toml`. Every knowledge store gets the same processors regardless of what that store actually needs.

DR6 already designed a fix — named pipelines, per-store bindings, and a versioned activation policy — and DR7 already designed the precedence/fallback rules. The 2026-08-08 code-verification pass (ADR §16.1) found that DR6's schema and resolution code are fully built and already wired into per-record processing (`ControlService.handleEvent` → `resolveProductionPlanFacts` → `ResolveProductionPipelineResolution`), but no real policy content has ever been authored: only placeholder pipelines (`legacy_default`, `store_default`, `request_override`) exist. A bootstrap policy has been active since an early migration, but no real bindings have ever been authored under it — `kb.pipeline_bindings` was empty.

This design authors real content into that existing mechanism. It does not add new resolution logic, new tables, or a new runtime code path.

## 2. Decision

Author "Doc Processing Policies" declaratively in `config.local.toml`, and add one small seed tool that reads them and upserts the existing `kb.pipelines` / `kb.pipeline_bindings` / `kb.pipeline_policies` tables through the existing store code — the same "config file is the source of truth, a seed tool applies it to the DB" pattern `server/cmd/ontology-seed` already established for ontology content.

Term mapping onto the existing schema:

| This design's term | Existing schema |
|---|---|
| Doc Processing Policy | one `kb.pipelines` row: `name`, `processors[]` |
| "associate a policy with a knowledge store" | one `kb.pipeline_bindings` row, `binding_kind='store_default'`, `ks_store_id=<id>` |
| default policy | one `kb.pipeline_bindings` row, `binding_kind='store_default'`, `ks_store_id=NULL` |
| (all of the above) | held inside one new, activated `kb.pipeline_policies` version |

`kb.pipeline_bindings` has no `scope_kind`/`scope_key` columns. "Scope" (`system`/`knowledge_store`/`user`/`tenant`/`document`) is derived at read time by a SQL `CASE` expression over which of `ks_store_id`/`user_id`/`tenant_id`/`input_record_id` is populated on the row (see `server/api/doc-processing/pipeline_bindings.go` and `policy_compile.go`), not a stored column.

A pipeline's `processors` list is an **allow-list intersected against the request**, not a full override: `static_analyzer`/`chunking`/`extract_doc_metadata` always run regardless of policy (existing `applyPolicyFilter` behavior, unchanged by this design).

## 3. Config format

```toml
[doc-processing-policy-no-entities-relations]
description = "Default policy. All processors except entities and relations."
is_default = true
processors = [
    "extract_metrics",
    "extract_provisions",
    "extract_semantic_projections",
    "generate_topics",
    "generate_scene_blocks",
    "extract_inventory_items",
]

[doc-processing-policy-all]
description = "All doc processors, including entities and relations."
is_default = false
processors = [
    "extract_metrics",
    "extract_provisions",
    "extract_semantic_projections",
    "extract_entity",
    "extract_relation",
    "generate_topics",
    "generate_scene_blocks",
    "extract_inventory_items",
]

[doc-processing-policy-bindings]
Research = "doc-processing-policy-all"
```

Rules:

- Every `[doc-processing-policy-*]` section is one policy. The stored `kb.pipelines.name` is the section-name suffix after `doc-processing-policy-` (e.g. `no-entities-relations`, `all`) — short, and matches what shows up in `kb.pipelines`/plan output.
- Exactly one section must have `is_default = true`. Zero or more than one is a hard error at seed time — no silent tie-break.
- `processors` must be non-empty, and every entry must match a real registered processor name (validated against the same registry `productionProcessorSpecs` uses). An unknown name is a hard error at seed time, not a runtime surprise.
- `[doc-processing-policy-bindings]` maps a knowledge-store name (matched against `kb.knowledge_store.ks_name`, same convention as the existing `default_knowledge_store` key) to a policy's section-name suffix. An unknown store name or unknown policy name is a hard error at seed time.
- A store not listed in `[doc-processing-policy-bindings]` has no store-level binding and falls through to the `system`-scope default binding.
- A policy's `description` field maps directly onto `kb.pipelines.display_name` (the only free-text field that table has). No other TOML field is needed to populate it.
- The seed tool never deletes a `kb.pipelines` row. Removing a `[doc-processing-policy-*]` section just means that pipeline is no longer referenced by the next activation — the row itself is left in place (harmless: only pipelines reachable through an active policy's bindings affect resolution). This avoids ever breaking a still-referenced binding from a prior policy version.

## 4. New tool: `server/cmd/doc-processing-policy-seed`

Modeled directly on `server/cmd/ontology-seed`.

**Behavior on each run:**

1. Load and validate `config.local.toml`'s `[doc-processing-policy-*]` sections and `[doc-processing-policy-bindings]` table per the rules in §3. Fail closed (exit non-zero, no DB writes) on any validation error.
2. Resolve each bound knowledge-store name to its `kb.knowledge_store.id`.
3. In one transaction:
   - Upsert each `kb.pipelines` row by `name` (insert if new; update `display_name`/`processors` if the section already exists).
   - Insert a new `kb.pipeline_policies` row (new version).
   - Insert one `system`-scope `kb.pipeline_bindings` row pointing at the `is_default` pipeline.
   - Insert one `knowledge_store`-scope `kb.pipeline_bindings` row per `[doc-processing-policy-bindings]` entry.
   - Activate the new policy version (same activation logic `POST /kb/pipeline-policies/:id/activate` uses).
4. Print a summary: pipelines created/updated, bindings written, activated policy version/id.

**Idempotency:** safe to re-run after editing the file. Each run creates a new `kb.pipeline_policies` version and activates it (matching the existing DR2/DR6 "append-only versions, activation is a separate audited pointer" invariant) rather than mutating a previously-activated version in place.

## 5. Rollout / verification

1. Fix the TOML syntax bug already in `config.local.toml` (stray `]` after each `processors` array closes) — blocks parsing entirely otherwise.
2. Run the seed tool against a dev database. Confirm the printed summary matches expectations.
3. With `DOC_PIPELINE_PLAN_ONLY` left at its default (unset = plan-only), process a document from an unbound store and one from `Research`. Inspect the persisted plan (`kb.doc_process_plans` / `GET /kb/doc-proc-plans/latest`) and confirm it resolves to `no-entities-relations` and `all` respectively, **without any change in which processors actually ran** — plan-only mode never enforces.
4. Only once step 3 looks correct, set `DOC_PIPELINE_PLAN_ONLY=false` in the target environment and re-verify: the unbound-store document's actual processor run should now exclude `extract_entity`/`extract_relation`.

## 6. Testing

- Unit tests for TOML parsing/validation: missing/duplicate `is_default`, unknown processor name, unknown store name, unknown policy name in bindings, empty `processors`.
- `go-sqlmock` tests for the seed transaction: upsert-vs-insert branch for `kb.pipelines`, correct `scope_kind`/`scope_key` on both binding kinds, new-version-per-run activation, and full rollback on any mid-transaction failure.
- One integration-style test (or manual step, if a live DB isn't available in CI for this tool) confirming a second run against the same config is a no-op change to `kb.pipelines` content but still produces a new activated policy version.

## 7. Explicitly out of scope for this pass

- Per-processor gates/predicates inside a policy (`kb.pipeline_rules` stays empty).
- Frontend admin pages for policies/bindings (planned later).
- Hot-reload — re-running the seed tool is the update mechanism.
- Binding scopes other than `system` and `knowledge_store` (no document/user/tenant-level overrides).
- Deleting/deactivating orphaned `kb.pipelines` rows (see §3's last rule).
