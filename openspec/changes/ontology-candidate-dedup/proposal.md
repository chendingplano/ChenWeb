## Why

`kb.ontology_candidates` only dedups by an exact-match SHA-256 fingerprint over the full proposed
payload, so two candidates describing the same real-world term — differing only in wording, or in
which name is `label` vs. an alias — land as separate rows instead of being flagged for review.
Root-caused in `KnowledgeStore/doc-repo/bugs/202608/2026081101-bug-ontology-candidates-fingerprint-dedup.md`:
document 416 produced two `metric_definition` candidates for the same metric because `label` and
its sole alias swapped between two extraction passes, changing both the fingerprint and the
derived `term_id`. If both are independently approved, they promote into two separate
`kb.ontology_terms` rows with no error — silent duplicate governed vocabulary.

## What Changes

- `CandidateStore.CreateCandidate` computes a second, deterministic **identity key** at insert
  time — `module_id + candidate_kind + term_kind (from payload, when present) + normalized sorted
  set of {label} ∪ aliases` (lowercase, trimmed, width-folded) — independent of the exact-payload
  fingerprint, which is unaffected by wording changes or label/alias reordering.
- New indexed column `identity_key` on `kb.ontology_candidates` (goose migration).
- On insert, if any existing row with `status NOT IN ('rejected', 'superseded')` shares the same
  `identity_key`, the insert still proceeds (no blocking, no auto-merge) but `candidate_matches`
  — an existing, currently-unpopulated JSONB column on this table — is set on the new row (and
  appended on the matched row) recording the match: matched candidate id, `matched_on:
  "identity_key"`, timestamp.
- Centralized in `CreateCandidate`, so every writer benefits without individual changes:
  `extract_metric_definitions`, `extract_test_methods`, `extract_metrics`'s inline harvest, and the
  manual `POST /kb/ontology/candidates` endpoint.
- Out of scope, explicitly: embedding/paraphrase-level fuzzy matching (the `keywords` family's
  `semid` + pgvector pattern is a reasonable future upgrade if exact-set matching proves
  insufficient, not part of this change); any reviewer UI (none exists yet for this table —
  confirmed separately, tracked as its own gap); changing `promote.go`'s promotion-time uniqueness
  behavior; auto-merging or blocking inserts based on a match.

## Capabilities

### New Capabilities
- `ontology-candidate-dedup`: `kb.ontology_candidates` gains a wording-invariant identity key that
  flags likely-duplicate term/label/mapping/axiom candidates to a curator via the existing
  `candidate_matches` field, without blocking insertion or auto-merging.

### Modified Capabilities
(none — no existing spec covers `kb.ontology_candidates` write/dedup behavior; the current
fingerprint-only behavior is implicit in code, not spec'd)

## Impact

- **Backend code**: `server/api/ontology/candidates/candidates_store.go` (`CreateCandidate`), a new
  identity-key helper alongside `fingerprint.go`.
- **Database**: one goose migration adding `identity_key TEXT NOT NULL` + a non-unique index (not
  a uniqueness constraint — this is a soft signal, not a hard dedup gate) to `kb.ontology_candidates`.
- **Downstream**: `candidate_matches` becomes populated by a live, reachable code path for the
  first time (previously only writable by the dead, unwired `semid.TermFamily.ResolveCandidate`).
  No consumer currently reads it — no reviewer UI exists for this table (confirmed in the bug doc)
  — so this is inert until one does, but it produces correct data for when that UI exists rather
  than requiring a backfill later.
- **No effect** on `normalize_assertions`/`associate_semantics`/`project_semantics` — confirmed
  these never read `kb.ontology_candidates` (separate pipeline over `kb.metrics`/`kb.provisions`).
- **No frontend changes** — no UI currently consumes this table's API.
