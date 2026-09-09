## Context

The knowledge base already holds every fact the Product Metric Reviewer needs; what it lacks is a
composition of them. The relevant substrate, as it exists today:

| Table | What it gives the app |
|---|---|
| `kb.products` | Per document, which products the document is *about*, with `relation_type` ∈ {`scope`, `regulated_object`, `requirement_target`, `component_of`, `contains_product`, `storage_requirement`, `maintenance_requirement`, `usage_condition`, `installation_requirement`, `testing_requirement`, `certification_requirement`, `risk_source`, …}, plus `canonical_name(_en)`, `related_products`, `category_paths`, `evidence_quote`, `evidence_lines`. Already hybrid-indexed as the `product` partition of `kb.search_artifacts`. |
| `kb.metrics` | `metric_name`, `metric_subject`, `subject_concept_id` (FK → `kb.keyword_concepts`, scope `metric_subject`), `metric_definition_term_id`, `metric_context`, `source_line_spans`, `input_record_id`. Hybrid-indexed as the `metric` partition. |
| `kb.object_nodes` / `kb.artifact_objects` | Cross-document reconciled object identity, with `normalized_names`, aliases, and a `vector(1536)` embedding. |
| `kb.semantic_assertions` | Accepted `core:part_of` / `core:component_of` edges between `object_node`s, produced by `extract_product_structure` from explicit relations. |
| `kb.doc_facet_values` (+ `kb.doc_facets`) | Document kind as the governed facet `document.doc_kind` written by the tier-3 `classify_document` processor (values `da:doc_kind_standard` / `_specification` / `_regulation` / `_manual`), with `kb.doc_facets.input_doc_type` as the coarse fallback; plus authority hint (GB/ISO/IEC/…) and normative status. |
| `kb.search_artifacts` (+ `kbsearch`) | Partitioned lexical `tsvector` + pgvector store with an established RRF fusion query used by `SearchAllArtifacts` and `classcontractsearch.MatchSimilar`. |

Two constraints shape the design. First, the part graph in `kb.semantic_assertions` is **sparse** —
`extract_product_structure` only converts *explicit* `part_of` / `component_of` relations with both
endpoints reconciled, so a cold product yields few or no edges. Second, `CLAUDE.md` §1.2 is
explicit about simplicity: the app must be a reader with its own small write footprint, not a new
pipeline stage.

## Goals / Non-Goals

**Goals:**

- Answer "which metrics relate to product P?" with recall across P's part hierarchy and P's
  lifecycle aspects, not just P's name.
- Make every result explainable: which scope node matched, by which path, in which document, at
  which lines.
- Make the product scope a durable, curatable, reusable object — a reviewer's corrections must
  survive and improve the next run.
- Report absence as a first-class result: scope nodes with zero metrics are a finding.
- Keep the retrieval contract artifact-type-agnostic so provisions and inventory items follow
  without redesign.

**Non-Goals:**

- No new extraction or LLM pass over documents. The app never writes to `kb.metrics`,
  `kb.products`, `kb.semantic_assertions`, or any pipeline table.
- No governed-ontology promotion. Scope nodes are app-local; they may *reference* governed terms
  but never create or promote them. (A future change may harvest curated profiles into
  `kb.ontology_candidates`; explicitly out of scope here.)
- No metric comparison, normalization, or conflict detection between metrics — that is the
  class-contract machinery's job.
- No cross-tenant profile sharing beyond what `kb.inputs.tenant_id` already scopes.
- v1 registers only `artifact_type = 'metric'`.

## Decisions

### D1 — A product scope profile is a persisted, versioned tree, not a query-time expansion

**Decision.** Resolving a product into its structure is a separate, persisted step
(`kb.product_profiles` + `kb.product_profile_nodes`) that a run *references*, rather than work
redone inside each retrieval call.

**Why.** The decomposition is the expensive, judgement-heavy, and most-wrong-able part. If it were
recomputed per query it would be non-deterministic between runs, un-curatable, and impossible to
diff; two runs a week apart would differ for reasons unrelated to new documents. Persisting it
makes a run reproducible (`run → profile_version → node set`), lets a reviewer's correction pay off
forever, and lets one profile serve many runs and many artifact types.

**Alternatives considered.** *Query-time LLM expansion* — rejected: unstable, uncitable, no
curation surface. *Graph-only traversal of `core:part_of`* — rejected as the sole source: correct
but empty for a cold product, which is exactly the case the app is for. It survives as one of the
three construction passes.

### D2 — Three-pass construction: propose → ground → expand

**Decision.**

1. **Propose.** One LLM call takes `{product_name, product_description?, seed_document_ids?}` and
   returns a decomposition tree — modules, parts, parts of parts — with a per-node rationale and
   confidence. The prompt lives in `prompts/prompt-product-structure-v1.md`.
2. **Ground.** Every proposed node is resolved, in this order, and stops at the first confident hit:
   - `kb.object_nodes` via `normalized_names` exact/alias match, then embedding similarity above a
     configured threshold → sets `object_id`;
   - `kb.keyword_concepts` in scope `metric_subject` via `pref_label` / alias → sets `concept_id`;
   - `kb.ontology_terms` labels → sets `term_id`.
   A node that grounds to nothing stays in the tree with `grounding = 'ungrounded'` and is matched
   lexically only. Grounding state is recorded per node, never silently dropped.
3. **Expand.** For every node with an `object_id`, traverse accepted `core:part_of` /
   `core:component_of` assertions (both directions, `status = 'accepted'` only) and
   `kb.products` rows with `relation_type IN ('component_of','contains_product')` whose
   `canonical_name` resolves to the same object. Newly discovered parts enter as
   `origin = 'graph_expanded'` with the source edge recorded.

Passes 2 and 3 iterate to a configured depth (default 3) and node budget (default 200).

**Why.** Pass 1 gives recall on a cold product; pass 2 converts strings into identities so that
retrieval can match by `subject_concept_id` rather than by `ILIKE`; pass 3 supplies the parts a
model would never guess but the corpus actually documents. Each pass compensates for the others'
characteristic failure. Origin is stored per node so a reviewer can see *why* a part is in the tree
and trust the grounded ones more than the proposed ones.

### D3 — Aspects are scope nodes, and they join to `relation_type` rather than to prose

**Decision.** Lifecycle aspects (`storage`, `transport`, `maintenance`, `usage_environment`,
`safety`, `packaging`, `disposal`, `emc`) are nodes of kind `aspect` attached to the root, drawn
from a configured vocabulary in `product-review.local.toml`. Each aspect declares the
`kb.products.relation_type` values it corresponds to, e.g.
`storage → ["storage_requirement"]`, `maintenance → ["maintenance_requirement"]`,
`usage_environment → ["usage_condition","installation_requirement"]`,
`safety → ["risk_source","certification_requirement"]`.

**Why.** The product-extraction prompt already classifies every product mention into exactly these
relation types, and that classification is persisted per document. Aspect matching therefore
reduces to an indexed join on `(product identity, relation_type)` — deterministic, cheap, and
explainable — instead of a second semantic search over free text. Aspects with no natural
`relation_type` mapping (e.g. `emc`) fall back to lexical/vector matching on the aspect's own
labels, and the config marks which mode each aspect uses.

**Alternatives considered.** Hard-coding the aspect list in Go — rejected: it is per-domain copy,
and `CLAUDE.md` §2 plus the `doc-review.local.toml` precedent put i18n'd vocabulary in config.

### D4 — Retrieval is two paths unioned, and the document-first path keeps unattributed metrics

**Decision.** A run computes:

- **Document scope set** — documents scored by: (A) a `kb.products` row whose identity matches a
  scope node, weighted by `relation_type` (`scope`/`regulated_object`/`requirement_target` highest);
  (B) RRF hybrid search over the `product`, `summary`, and `topic` partitions of
  `kb.search_artifacts` using the node's labels; (C) a **boost** — never a filter — for
  `doc_kind = standard|specification|regulation`. Each document keeps its match reasons.
- **Path D (document-first)** — every metric of every document in the scope set.
- **Path E (direct)** — metrics whose `subject_concept_id` equals a scope node's `concept_id`, or
  whose subject matches a node via RRF over `kb.search_artifacts_metric`, **with no document
  restriction**.

Results are unioned and deduped on `(artifact_type, artifact_id)`; a hit found by both paths keeps
both, in a `paths` array. Each hit is then tiered by *what it matched*, independent of *how it was
found*: `direct` (subject matches the root product) > `part` (subject matches a part/module node) >
`aspect` (subject or context matches an aspect node) > `document_scope` (no subject match, but its
document is in scope).

**Why.** Path D alone implements the user's stated pivot and gets `Display Luminance` out of a
ventilator standard; Path E alone gets it out of a display datasheet that never says *ventilator*.
Neither subsumes the other. Keeping `document_scope` hits rather than discarding them is what makes
the document-first pivot lossless — a metric in a ventilator standard is relevant even when its
subject could not be attributed to any node — but the tier keeps them ranked below attributed
hits so they never drown the answer.

**Trade-off accepted.** `document_scope` is a large tier: a single standard can contribute hundreds
of metrics. It is capped per document and per run by config, and the UI collapses it by default.

### D5 — Results are keyed `(artifact_type, artifact_id)`, the `kb.search_artifacts` key

**Decision.** `kb.product_review_results` stores `artifact_type` + `artifact_id` (e.g.
`metric` / `1234_mtc_7`, per `kbsearch.BuildArtifactID`) plus `source_row_id`, not a
`metric_id` FK. A small Go registry maps an artifact type to its store adapter: how to list by
document, how to match by subject concept, which search partition to fuse, and which columns to
project into a result row.

**Why.** It is the key the hybrid index already uses, so results join back to
`kb.search_artifacts` for labels, snippets, and line spans without per-type code in the read path.
It is also what makes "or any other artifacts in general" a configuration change: registering
`provision` means writing one adapter, not touching the schema, the run controller, the report, or
the UI table.

### D6 — Request / run split, mirroring doc-review

**Decision.** `kb.product_review_requests` holds intent (profile id + version, artifact types,
filters, notes, requester); `kb.product_review_runs` holds one execution (status, timings, counts,
error, report). Re-run creates a new run under the same request, and the run read endpoint can diff
against the request's previous run.

**Why.** The doc-review tables (`kb.doc_review_requests` / `_runs`, ADR 2026062804) already
established this split in this codebase for exactly this reason, and re-run-after-ingest is a
primary use case here — "what became known about ventilators this month" is a diff of two runs of
one request.

**Deviation from doc-review, deliberate.** doc-review has a separate `kb.doc_review_reports` table
because reports are re-rendered from templates. This app has one report per run, so the report is
`report_json JSONB` + `report_md TEXT` on the run row. One fewer table for no lost capability.

### D7 — Coverage gaps are computed, not inferred by the reader

**Decision.** The run report contains a per-node coverage table — node, kind, grounding state,
metric count, document count — and an explicit `gaps` list of accepted nodes with zero hits.

**Why.** For a reviewer preparing a standards-conformance file, "no metric found for the humidifier
module" is more actionable than any of the metrics that *were* found. It is also the honest way to
expose the app's own recall limits: a gap is either a real corpus gap or a grounding failure, and
the node's `grounding` state tells the reviewer which.

### D8 — LLM usage is bounded and confined to profile construction

**Decision.** Exactly one LLM call per profile build (the decomposition), plus at most one optional
call for aspect mapping of ungrounded aspect labels. Retrieval, scoring, tiering, and reporting are
pure SQL and Go. Embeddings reuse `kbsearch`'s existing embedder and are subject to
`kbsearch.SemanticSearchEnabled`.

**Why.** Retrieval must be reproducible and fast enough for interactive use; a run over a curated
profile should be re-runnable without cost. It also keeps the failure surface small — if the
embedder is disabled, the app degrades to lexical matching rather than failing.

## Risks / Trade-offs

- **The proposed decomposition is wrong or generic** ("a ventilator has a housing") →
  Grounding marks such nodes `ungrounded`, expansion adds nothing under them, and they produce
  gaps rather than false hits. Curation is the fix, and it persists. The report shows the
  grounded/ungrounded split so a reviewer can judge the tree's quality at a glance.

- **`document_scope` floods the result set** → per-document and per-run caps in config, tier
  collapsed by default in the UI, and the coverage table counts attributed vs unattributed hits
  separately so the flood never hides a gap.

- **A part name is ambiguous across domains** ("valve", "display", "battery") → grounding to
  `object_id` / `concept_id` disambiguates where the corpus supports it; where it does not, the
  node stays lexical and its hits are marked lower-confidence. `kb.object_nodes` already has a
  `pending_review` / `ambiguous` reconcile state and an admin surface; the app surfaces that state
  on the node rather than silently picking a sense.

- **The part graph stays sparse, so pass 3 adds little** → accepted for v1: passes 1 and 2 carry
  the app. Every graph-expanded node is labelled as such, so the value of the graph is measurable
  from run to run, and improving `extract_product_structure` coverage becomes a measurable win
  rather than a guess.

- **Profile drift across runs** → runs pin `profile_version`; editing a profile bumps its version
  and never mutates a completed run's node set. A diff between runs therefore separates
  "the corpus changed" from "the scope changed".

- **Cost of embedding many node labels** → node embeddings are computed once at grounding time and
  stored on the node row, not per run.

## Migration Plan

Additive only. Six new `kb` tables, one config file, two prompt files, one route block, one page.
No backfill: profiles are created on demand, and the first run over a new profile is the backfill.
Rollback is dropping the six tables and the route block; nothing outside the app reads them.

Because the dev server runs `mise dev` with air hot-reload, migrations apply on the first rebuild
after a file lands — see the workspace note about `project_db_migration` and half-edited migration
files.

## Open Questions

- **Aspect vocabulary scope.** The eight aspects above are a medical-device-flavoured starting set
  drawn from the `relation_type` enum. Whether a tenant needs its own aspect list (per-tenant TOML,
  as `site-config` already does for copy) is deferred until a second domain appears.
- **Profile reuse across product families.** A "ventilator" profile and an "anaesthesia machine"
  profile share most parts. Cloning is trivially supported; whether profiles should *inherit* is
  left open — inheritance changes the versioning model and has no demand yet.
- **Harvesting curated profiles back into the ontology.** A reviewer-curated, grounded part tree is
  exactly the input `kb.ontology_candidates` wants. Deliberately deferred (D-Non-Goal 2) so the
  app cannot corrupt governed vocabulary before its own quality is measured.
