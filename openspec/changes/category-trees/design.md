## Context

Devdoc `KnowledgeStore/doc-repo/devdocs/202610/2026102003-devdoc-categories.md` (the source of
the requirements) replaces per-artifact LLM category paths (ADR 2026071901,
`kb.artifact_categories`, `category_name` edges in `kb.artifact_connections`) with pre-built
category trees and decision-model filing. Extractors already store LLM category proposals with
their artifacts (`kb.metrics.category_paths` / `category_paths_en`), and the codebase already
has decision-model clients: `decisionmodel.ProviderConfig` maps a `.models.toml` entry to Jev
(`decision-model`) or the logprob emulation `jev_emulated` (`llm`), both answering typed
`noul` / `choice` questions with probabilities (see `metric_open_value_decision.go`).

The devdoc leaves several things to be "specified before implementation". This design fixes
defaults for each so the mechanism can be built and evaluated; all numeric limits are env vars.

## Goals / Non-Goals

**Goals:**
- Storage, change log, validation, revisions and replay exactly as devdoc §3, §4, §8.
- The two-stage classification of §5 with the outcome rules of §5–§7, publication of §6, and
  stale marking of §4.3.
- An HTTP API to author trees, run classification, and work the review queue.
- `metrics` as the first artifact type with a working source.

**Non-Goals:**
- UI pages (tree browser, review queue). The API is designed so a page can be added later.
- A doc-processing pipeline processor; classification is triggered through the API.
- The initial vocabulary, model selection, the evaluation sample, and retirement of ADR
  2026071901 data. Existing category code stays as is.
- Automatic checking that a child's definition is narrower than its parent's (§3.2) — that is a
  semantic judgement; it is left to tree authors and review.
- `skos:related` (optional in the devdoc; nothing reads it yet).

## Decisions

### D1. One package, pure core + thin SQL

`server/api/categorytree` holds:
- `tree.go` — in-memory `Tree` (nodes by ID, children index) and node types.
- `validate.go` — the §3.3 checks over a `Tree`.
- `changeset.go` — applies operations to a `Tree`, maintains "others" leaves, and yields the
  log entries and the stale-marking effects. Pure; unit-tested without a database.
- `store.go` — SQL: load current tree, apply a change set in one transaction, revisions,
  replay, bindings, classifications.
- `classify.go` — the verification walk over a `Tree` with a `Decider` interface. Pure.
- `decider.go` — the Jev/jev_emulated `Decider`.
- `source.go` — `ArtifactSource` and the `metrics` source.
- `service.go` — runs classifications in the background.
- `handler.go` — Echo handlers.

Alternative considered: put it inside `doc-processing`. Rejected: that package is already very
large and the tree is not a pipeline stage. The package reuses `docprocessing.LoadPromptByRef`
and a newly exported `docprocessing.ResolveModelsFilePath` (one-line export) to load prompts and
`.models.toml` the same way the extractors do.

### D2. Change sets are applied in memory, then written

A change set locks the tree row (`SELECT … FOR UPDATE` on `kb.category_trees`), loads the
current non-retired nodes, applies the operations to the in-memory tree, runs validation, and
only then writes: the log entries (one per affected node, `seq` = `current_seq` + 1 …), the
node rows (insert / update / `retired_at`), stale marks on bindings, and the new
`current_seq`. Any failure rolls back everything, so every `seq` in the log is a valid tree.
Trees are expected to hold hundreds to a few thousand nodes, so loading the whole tree is cheap.

Each log entry stores `before` and `after` as the full node JSON (null for add / delete), so
replay is "start empty, for each entry set node = after (or retire on delete)". The same
`Tree` and validator run during replay; the API's "view at revision" returns the replayed tree.

The initial tree is the tree's first change set (`seq` 1..n), created by
`POST /kb/category-trees` from a nested JSON document, as devdoc §8.1 says.

### D3. "Others" leaves are maintained by the engine, logged as ordinary changes

After the caller's operations are applied, the engine reconciles reserved leaves and appends
their changes to the same change set (author `system`, reason naming the cause):
- an inner node without an "others" child gets `<P>-others` (labels "Others"/"其他", the
  generated definition in `en` and `zh`);
- a node whose ordinary children are all gone loses its "others" child (it is a leaf again);
- a renamed P renames `<P>-others`; changed P labels regenerate the "others" definition.

Callers may not add, move, update or delete "others" nodes, nor give them children. "Others"
nodes are exempt from the preferred-label uniqueness check (every one is labelled "Others").

### D4. Operations

`add` (new node with parent; ID is a fresh UUID), `delete` (retires the node and its whole
subtree, one log entry per node — deleting a subtree is the common intent and avoids forcing
callers to order deletes), `move` (new parent; not under itself, a descendant, or an "others"
leaf; the root cannot be moved or deleted), `update` (any of canonical name, labels,
definitions, scope notes, source, matches). Validation runs once after all operations, so a
change set may pass through invalid intermediate states (e.g. add then rename).

Node IRI = tree namespace + node ID. External / cross-tree links are stored in a `matches`
JSONB list of `{type: exact|close, iri}`; validation rejects `exact` to a node of the same tree.

### D5. Stale marking (devdoc §4.3)

Computed from the change set's entries against the tree before and after:

| Entry | Bindings marked stale |
|---|---|
| update changing definitions or scope notes | on any node in the updated node's subtree |
| node gains its first ordinary child (add or move) | on that node (it was a leaf) |
| inner node gains an ordinary child (add or move) | on that node's "others" leaf |
| delete | on the deleted nodes |
| labels / source / canonical name / matches only; move | none (ancestors are derived) |

A binding keeps pointing at its node; `stale = true` and `stale_change_id` record why. A stale
binding stays published until reclassification replaces it.

### D6. Verification walk

The walk follows devdoc §5 steps 1–6 over the current tree (whose `seq` is recorded):

- Breadth-first queue starting at the root. A dequeued leaf joins the proposed direct set. For an
  inner node, ordinary children are split into batches of at most R (`CATEGORY_BATCH_SIZE`,
  default 20); each batch is one decision request, retried up to 2 times on error.
- Per child the decider returns P(member). `p >= CATEGORY_MATCH_MIN_P` (default 0.7) is a
  match, `p <= CATEGORY_NOMATCH_MAX_P` (default 0.3) is a non-match, anything between is
  uncertain. Only supplied child IDs are accepted; a missing answer is uncertain.
- All batches of a parent combined: any match → enqueue each matched child (deduplicated);
  all definite non-matches → the parent's "others" leaf joins the proposed set; otherwise the
  parent is unresolved and none of its children are enqueued.
- The walk continues over other branches so the record holds as much evidence as possible, but
  stops issuing requests once `CATEGORY_MAX_REQUESTS` (default 60) is reached.
- Result status: every parent resolved → `published` (bindings written); any request failed
  after retries → `failed`; otherwise uncertain or budget exhausted → `in_review`. The reason is
  stored. Failed / in-review attempts never touch the published bindings.

Batches of one parent run concurrently (bounded at 4); artifacts of a record run with
concurrency 4. `CATEGORY_DECISION_MODEL` names the `.models.toml` profile.

### D7. Decision request shape

One Jev request per batch: state JSON = `{policy, artifact, proposals, parent, categories}`
where `categories` maps short keys `c1…cN` to `{id, label, alt_labels, definition, scope_note}`
in the artifact's language (falling back to `en`). One `noul` question per key, its
instruction rendered from the question prompt with the key substituted. Prompt files:
`prompts/prompt-category-membership-policy-v1.md` (state `policy`) and
`prompts/prompt-category-membership-question-v1.md` (`{{key}}` placeholder). Short keys keep
UUIDs out of the questions; the mapping back to IDs is checked so only supplied IDs survive.

The artifact language is the `ArtifactSource`'s choice; the metrics source uses `zh` when the
metric name contains CJK characters, else `en`.

### D8. Publication and lifecycle

Publishing replaces all bindings of `(artifact_type, artifact_id)` and marks the
classification `published` in one transaction. Ancestors are never stored; the members of a
category (direct or through descendants) are found with a recursive CTE on `parent_id`.

`POST /kb/category-classifications/run {artifact_type, record_id}` lists the record's current
artifacts from the source, deletes bindings and classifications of that record and type whose
artifact no longer exists (re-extraction replaced them), then classifies each artifact in the
background. `POST …/reclassify-stale` reclassifies artifacts that have stale bindings.

Review queue: classifications with status `failed` or `in_review` that are the latest attempt
for their artifact. Reviewer actions: `retry` (new attempt), `close` (status `unclassified`;
publishes nothing), `resolve` (reviewer supplies leaf node IDs; validated to be current leaves
of the tree, published with the reviewer recorded as the decision source).

### D9. API

All routes under the authenticated `/api` group; the acting user ID is the change author.

- `GET /kb/category-trees`, `POST /kb/category-trees` (create with initial nodes)
- `GET /kb/category-trees/:name?revision=&seq=` (current, or replayed)
- `POST /kb/category-trees/:name/change-sets`, `GET /kb/category-trees/:name/changes`
- `GET|POST /kb/category-trees/:name/revisions`
- `GET /kb/category-trees/:name/nodes/:node_id/members?direct=`
- `GET /kb/category-trees/:name/others-report`
- `GET /kb/category-tree-types`, `PUT /kb/category-tree-types/:artifact_type`
- `POST /kb/category-classifications/run`, `POST /kb/category-classifications/reclassify-stale`
- `GET /kb/category-classifications?artifact_type=&record_id=&status=`
- `GET /kb/category-classifications/review`, `POST /kb/category-classifications/:id/review`
- `GET /kb/category-bindings?artifact_type=&artifact_id=` (direct + ancestors with paths)

## Risks / Trade-offs

- [A missed branch near the root hides all its descendants] → thresholds put borderline
  answers into review instead of "others"; every decision (probabilities, model, prompt
  refs, seq) is stored in the classification evidence for review and evaluation.
- [Uncertain answers may flood the review queue] → thresholds are env vars; tune them with the
  evaluation sample the devdoc calls for.
- [Whole-tree load per change set] → fine at expected sizes; revisit if trees grow large.
- [Bindings of deleted artifacts linger until the record is re-run] → `run` cleans them up;
  `ON DELETE CASCADE` on `record_id` covers deleted documents.
- [Classification runs in-process in the background] → a server restart drops in-flight work;
  those attempts stay `pending` and are retried by re-running the record.

## Migration Plan

One goose migration creating the seven tables (no data). Rollback drops them. Nothing else
reads them, so deploy order does not matter. Before first use: create a tree via the API, bind
`metrics` to it, set `CATEGORY_DECISION_MODEL`.

## Open Questions

- The initial vocabulary and namespace for the metrics tree (devdoc §3.3).
- Default thresholds and budgets need the evaluation sample of devdoc §7.
- When to wire classification into the extraction pipeline, and the UI.
