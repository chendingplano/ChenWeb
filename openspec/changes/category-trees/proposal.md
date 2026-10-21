## Why

Extractors invent category paths artifact by artifact (`kb.metrics.category_paths`), so the
same idea lands under many slightly different names and paths that are hard to canonicalize
afterwards. Devdoc `2026102003-devdoc-categories` proposes the reverse: people prepare a
category tree per artifact type in advance, and a cheap decision model files each artifact
under existing categories only. This change implements that proposal.

## What Changes

- New `kb` tables (goose migration): `category_trees`, `category_tree_types`,
  `category_nodes`, `category_tree_changes`, `category_tree_revisions`,
  `category_classifications`, `category_bindings` (devdoc §8.1).
- New Go package `server/api/categorytree`:
  - tree change sets (add / delete / move / update node) applied atomically with the
    devdoc §3.3 validation, logged append-only, with automatic "others" leaves;
  - named revisions and replay of the log to view the tree at any revision;
  - stale marking of affected bindings per devdoc §4.3;
  - two-stage classification (LLM proposals already stored with the artifact, then a
    decision-model walk down the tree) with match / no-match / uncertain outcomes,
    batches of at most R children, budgets and a review queue;
  - atomic publication of direct leaf memberships; ancestors derived by recursive query.
- Artifact source for `metrics` (proposals read from `kb.metrics.category_paths_en` / `category_paths`
  and `metric_categories_en` / `metric_categories`;
  other artifact types plug in through the same interface later).
- HTTP API under `/api/kb/category-trees/...` to create trees, apply change sets, create
  and view revisions, bind artifact types, classify a record's artifacts, reclassify stale
  bindings, list a category's members and the review queue.
- New prompts `prompts/prompt-category-membership-policy-v1.md` (membership rules) and
  `prompts/prompt-category-membership-question-v1.md` (the per-category question).
- Not in this change: a UI, an extraction-pipeline processor, the initial vocabulary, and
  retirement of ADR 2026071901 data (`kb.artifact_categories`, `category_name` edges) —
  devdoc §7 keeps these as separate decisions. Existing category code is untouched.

## Capabilities

### New Capabilities
- `category-trees`: tree storage, change log, validation, revisions and replay, artifact-type
  binding, and stale marking of bindings.
- `category-classification`: proposal + decision-model verification walk, outcomes, budgets,
  review queue, publication of bindings, and the classification API.

### Modified Capabilities
- (none)

## Impact

- New migration in `project_migrations/`; new package `server/api/categorytree`; routes in
  `server/api/routes.go`; one new prompt file; env vars `CATEGORY_DECISION_MODEL`,
  `CATEGORY_MATCH_MIN_P`, `CATEGORY_NOMATCH_MAX_P`, `CATEGORY_BATCH_SIZE`,
  `CATEGORY_MAX_REQUESTS`.
- Uses the existing decision-model clients (`decisionmodel.ProviderConfig`, Jev / jev_emulated
  `noul` questions). No change to the shared library.
