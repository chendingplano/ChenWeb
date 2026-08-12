# Metric Merge Static-First Design

## Goal

Reduce expensive metric-merge LLM calls while preserving conservative handling
of genuinely ambiguous overlaps between newly enriched metrics and existing
`kb.metrics` rows.

## Current problem

In incremental (`force_clear=false`) extraction, exact duplicates are removed
first. Every remaining candidate that overlaps an existing metric by
`source_line_spans` is then placed into a transitive merge group and sent to the
merge-resolution LLM. This treats simple, statically distinguishable cases as
ambiguous and can create many unnecessary calls.

## Decision ladder

The merge classifier will apply these rules in order:

1. Exact duplicate: discard the new candidate.
2. No source-span overlap with any existing row: assign a new metric ID.
3. Source-span overlap with existing rows: use normalized structured fields to
   determine whether one existing row is a clear match. A clear one-to-one
   match is merged statically without an LLM call.
4. Remaining ambiguity: retain the existing connected-component behavior and
   send only unresolved groups to the merge-resolution LLM.

Static matching remains conservative. It must not merge when multiple existing
rows are equally plausible, or when the candidate's identifying fields conflict
with the possible matches. The existing LLM prompt and validation remain the
last-resort path.

## Data flow

`mergeMetrics` will classify each candidate after exact-duplicate filtering.
Candidates classified as added or statically merged are returned directly.
Only unresolved candidates and the existing rows they overlap will participate
in connected-component grouping and `PendingGroups`.

Static merges retain the existing metric identity and update the candidate's
changed fields using the same dirty-check/upsert path as LLM winners. No LLM is
used for keyword-concept or governed-term resolution; that remains the later
resolver-backed persistence step.

## Testing

Add deterministic unit coverage for isolated additions, unique static matches,
multiple distinct static matches, and genuinely ambiguous groups. Preserve the
existing tests covering exact duplicates, transitive grouping, and LLM winner
application. Verify the focused doc-processing package and related ontology
packages.

## Out of scope

- Changing the merge-resolution prompt or model.
- Changing candidate extraction or enrichment prompts.
- Changing keyword-concept/governed-term resolution.
- Adding database schema or migration changes.
