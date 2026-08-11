## Context

`CandidateStore.CreateCandidate` (`server/api/ontology/candidates/candidates_store.go:141-207`) is
the single insert path for `kb.ontology_candidates` (all writers — `extract_metric_definitions`,
`extract_metrics`'s inline harvest, `extract_test_methods`, the manual API — go through it). Its
only dedup signal is `Fingerprint` (`fingerprint.go:17-29`): SHA-256 over the canonicalized full
`proposed_payload` + `source_type` + `source_ref` + `module_id`, enforced by
`ON CONFLICT (fingerprint) DO NOTHING`. Any wording difference between two extraction passes over
the same document — including which of two synonymous names ends up in `label` vs. `aliases` —
produces a different fingerprint and a new row, even though the underlying candidate is the same.
Root cause and impact analysis: `KnowledgeStore/doc-repo/bugs/202608/2026081101-bug-ontology-candidates-fingerprint-dedup.md`.

Two payload shapes exist under `candidate_kind = 'term'` today: `metric_definition`
(`ontology_candidate_harvest.go:220-235`, has `label` **and** `aliases`) and `concept`
(`ontology_candidate_harvest.go:74-77`, has `label` only, no `aliases` key at all). `axiom`-kind
payloads (`ontology_candidate_harvest.go:84-87`) have neither — they're keyed by
`subject_term_id`/`predicate_term_id`/`object_term_id` instead, a structurally different identity
problem. `label`/`mapping`/`profile`/`profile_rule`/`module_change` candidate kinds have no writer
in this codebase today at all.

`kb.ontology_candidates.candidate_matches` (JSONB) already exists for "here's a possible match,"
but has exactly one writer, `semid.TermFamily.ResolveCandidate` (`termfamily.go:139-145`), which
has zero callers anywhere in the repo (confirmed by grep) — it's dead code. Its shape,
`json.Marshal(res.Matches)`, is an array of `semid.NodeCandidate{NodeID, KeyBundle, Method}` —
i.e., matches against **released governed terms** (`kb.ontology_terms`, keyed by term IRI), not
against other pending candidates. Since this writer is unreachable, there is no live data and no
reader to break by changing the column's shape.

## Goals / Non-Goals

**Goals:**
- `kb.ontology_candidates` gains a second, wording-invariant identity signal for `candidate_kind =
  'term'` rows, so two candidates naming the same concept (regardless of which name is `label` vs.
  an alias, or minor description/definition drift) are flagged to each other.
- The flag is soft: it never blocks an insert, never merges rows, never changes `status`. A
  curator (once a UI exists — none does today) decides what to do with a flagged pair.
- Works uniformly across every writer because it lives in `CreateCandidate`, not per-extractor.

**Non-Goals:**
- No fuzzy/paraphrase matching (embeddings, edit distance). The identity key is an exact match on
  a normalized *set* of names — it fixes the label/alias-swap case from the bug doc and plain
  case/whitespace variance, nothing fuzzier. If that proves insufficient in practice, the
  `keywords` family's `semid` + pgvector pattern (`server/api/ontology/keywords/`) is the
  reasonable next step, not part of this change.
- No identity key for `candidate_kind` other than `term` — `axiom`/`label`/`mapping`/etc. payloads
  don't carry a `label`/`aliases` shape, and no writer for those kinds currently exists to observe
  the problem for. Extending this to axioms (e.g. keying on
  `subject_term_id`+`predicate_term_id`+`object_term_id`) is a plausible future change, not this
  one.
- No CJK/full-width Unicode folding in normalization — trim + casefold only. The bug doc's
  reproduction case (label/alias swap) doesn't need it, and it's real complexity for a benefit not
  yet demonstrated as necessary.
- No reviewer UI. Confirmed separately that nothing consumes `/api/v1/kb/ontology/candidates*`
  today. This change makes `candidate_matches` carry correct data for when a UI exists; building
  that UI is a separate change.
- No change to `promote.go`'s promotion-time uniqueness behavior, and no auto-block/auto-merge on
  insert.

## Decisions

**1. Identity key formula, scoped to `candidate_kind = 'term'` only.**
`IdentityKey(candidateKind, moduleID string, payload []byte) string` (new file
`server/api/ontology/candidates/identity.go`, alongside `fingerprint.go`): returns `""` unless
`candidateKind == "term"`. When it applies: unmarshal `payload` for `label` (string) and `aliases`
([]string, may be absent — treated as empty), form the set `{label} ∪ aliases`, drop empty
strings, normalize each with `strings.ToLower(strings.TrimSpace(...))`, dedupe, sort. If the
resulting set is empty (no usable `label`), return `""` — key is skipped, same as today's
no-op behavior. Otherwise: `sha256(moduleID + "\x00" + termKind + "\x00" + strings.Join(sortedSet,
"\x1e"))`, hex-encoded — same construction style as `Fingerprint`, so it's a familiar pattern and a
bounded, indexable column.
`termKind` is read from the payload's `"term_kind"` field (present on every existing `term`-kind
writer) so that a `metric_definition` candidate and a `concept` candidate that happen to share a
label text don't collide — they're different kinds of thing.
*Alternative considered:* key on `label` alone (ignore `aliases`). Rejected — it's exactly the
signal the bug doc's reproduction case needs; the two runs' `label` values differed (发芽指数 vs.
种子发芽指数) and only the aliases-inclusive set was identical between them.
*Alternative considered:* include `definition`/`description` in the key (closer to full-payload
matching). Rejected — those are exactly the fields expected to vary in wording between extraction
passes; including them would reproduce the fingerprint's blind spot, just with a smaller surface.

**2. Storage: nullable `identity_key` column, non-unique index — not a uniqueness constraint.**
`identity_key TEXT` (nullable — `""` from Decision 1 is stored as `NULL`, matching how
`nullableString` already treats other optional columns in this file), plus a plain btree index for
the lookup in Decision 3. Deliberately **not** `UNIQUE` — unlike `fingerprint`, this is a
similarity signal a human reviews, not a hard identity guarantee (two legitimately different
metrics could coincidentally normalize to the same key; blocking on that would be worse than the
current silent-duplicate behavior).
*Alternative considered:* a partial unique index scoped to non-terminal statuses (blocks a second
`discovered` row with the same key). Rejected for this change — that's a real option for a
follow-up once there's evidence false positives are rare enough to tolerate hard blocking; starting
soft avoids ever silently dropping a genuinely-new candidate because of a same-name coincidence.

**3. Matching happens as a best-effort follow-up after the existing insert, not inside it.**
`CreateCandidate` is unchanged through its existing fingerprint insert-or-reuse flow
(lines 148-207). After that returns a **newly created** row (`Reused == false`) with a non-empty
identity key, `CreateCandidate` runs one additional query:
```sql
SELECT id FROM kb.ontology_candidates
WHERE identity_key = $1 AND id != $2 AND status NOT IN ('rejected', 'superseded')
```
For each match found, two single-statement `UPDATE`s append a match entry to `candidate_matches`
on both the new row and the matched row (jsonb array concat:
`COALESCE(candidate_matches, '[]'::jsonb) || $entry::jsonb`), so either candidate's page shows the
other. If `Reused == true` (fingerprint hit — truly identical payload, existing row returned), skip
this entirely; that case is already fully handled.
*Alternative considered:* wrap insert + match-detection + both updates in one transaction for
atomicity. Rejected for this change — `CandidateStore.DB` is a plain `*sql.DB`, and every other
method on this store (`TransitionStatus`, `DeferCandidate`, ...) already does read-then-write as
separate statements, not transactionally; matching that existing pattern is more consistent than
introducing the only transactional path in this file for a feature whose worst failure mode (a
crash between insert and match-detection leaves `candidate_matches` unpopulated until the next
matching candidate is inserted, or a manual re-trigger) is a missed soft signal, not corrupted
data.

**4. `candidate_matches` becomes a discriminated array; this change defines its first entry
type.**
Since the column has zero live writers and zero readers today, this change is effectively the
first to give it a real shape. Define it as a JSON array of entries, each carrying a `match_type`
discriminator:
```json
[{"match_type": "candidate", "candidate_id": 17, "matched_on": "identity_key", "detected_at": "2026-08-11T12:00:00Z"}]
```
`match_type: "term"` is reserved for `semid.TermFamily`'s shape if it's ever wired up (its
`NodeCandidate{NodeID, KeyBundle, Method}` would map to `{"match_type": "term", "node_id": ...,
"method": ...}`) — not built here, since that writer is unreachable, but the discriminator leaves
room for it without a future breaking change to this column.
*Alternative considered:* a separate new column (e.g. `duplicate_candidate_ids`) instead of
touching `candidate_matches`. Rejected — `candidate_matches` exists for exactly this purpose
("possible match, human decides"), has no live data to conflict with, and adding a second
similar-purpose column would be the kind of unrequested flexibility Simplicity First warns against.

## Risks / Trade-offs

- **[Risk] False positives: two genuinely different candidates share a label/alias set** (e.g. two
  unrelated metrics both informally called "load" in different sections) → **Mitigation:** the key
  is scoped by `module_id` + `term_kind`, and the result is a soft flag for human review, never a
  block or merge (Decision 2). Worst case is an unnecessary "possible duplicate" flag, not lost
  data.
- **[Risk] `candidate_matches` redefinition surprises a future `TermFamily` wiring effort** →
  **Mitigation:** Decision 4's discriminator is designed for that case up front; flagged explicitly
  so it isn't rediscovered the hard way later.
- **[Trade-off] Non-transactional match detection (Decision 3) can leave a match briefly
  undetected** if the process crashes between the insert and the follow-up query → accepted; this
  is a soft signal, and the next candidate sharing that identity key (or a one-off backfill query)
  would surface it. Not treated as data loss.
- **[Trade-off] No fuzzy matching** — paraphrased duplicates with no shared label/alias text still
  slip through, same as today. Explicitly deferred (Goals/Non-Goals) rather than solved partially.

## Migration Plan

- One goose migration (`project_migrations/`, next available timestamp after
  `20260811000004`): `ALTER TABLE kb.ontology_candidates ADD COLUMN identity_key TEXT;` +
  `CREATE INDEX idx_kb_ontology_candidates_identity_key ON kb.ontology_candidates (identity_key);`.
  No backfill of existing rows (including candidates 16/17 from the bug doc) — `identity_key` is
  computed at insert time only; existing rows keep `identity_key = NULL` until a follow-up backfill
  script is run, if wanted. Not required for this change to be correct going forward.
- Code ships as a normal deploy; under `mise dev` (air hot-reload) the migration applies
  automatically to the dev DB on save, per the workspace's known behavior (verify with
  `SELECT * FROM project_db_migration ORDER BY id DESC LIMIT 5` if in doubt).
- Rollback: revert the code (stops computing/matching on `identity_key`) and, if desired, drop the
  column via a follow-up down-migration — safe either way since nothing else depends on the column
  existing.

## Open Questions

- Whether to backfill `identity_key`/`candidate_matches` for existing rows (including 16/17) as a
  one-off script, or leave history as-is and only apply this going forward. Leaning toward "leave
  as-is" (Simplicity First) unless the user wants the existing duplicate pair specifically flagged.
- Whether `detected_at` in the match entry (Decision 4) should be a stored timestamp or left
  implicit (inferred from `modify_time`) — minor, left to implementation.
