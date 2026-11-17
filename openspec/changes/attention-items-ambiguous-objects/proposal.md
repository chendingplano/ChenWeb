## Why

When object reconciliation finds two or more `kb.object_nodes` candidates with the same top
score, it leaves the artifact object unlinked (`object_id` NULL, `reconcile_status =
'ambiguous'`) and logs `WARN object reconciliation ambiguous` (ChenWeb issue #18, 15 rows in the
2026-11-13 to 2026-11-16 production alert review). Unlinked artifacts are missing from the
object graph until someone drains the backlog. The only record of what tied is the alert log.
There is also no place to collect other data problems that need a person to decide.

## What Changes

- New table `kb.attention_items`. It holds data problems that need a human decision, one row
  per problem. Each row has a type that classifies the problem, an open / resolved / dismissed
  lifecycle, a dedupe key so repeats of the same problem update one row, the evidence captured
  when the problem was seen, and who resolved it and how. A closed problem that recurs is
  reopened with the new evidence, because a fixed problem recurring means the fix did not hold.
  The first type is `ambiguous-object-name`.
- Object reconciliation no longer leaves ties unlinked. Among the tied candidates it links the
  deterministic tie-break pick (`pickTieBreakCandidate`, already used by the bulk backfill):
  `reconcile_status = 'ambiguous_resolved'`, `ext_info.reconcile_method =
  'tie_break_deterministic'`. It records an `ambiguous-object-name` attention item listing all
  tied candidates and the pick. The LLM adjudicator (DR7), when configured, still runs first;
  only ties it does not resolve take the tie-break and become an attention item.
- Tie detection compares every candidate with the top score. Today it compares the first two
  candidates in query order, which is not score order.
- The `WARN object reconciliation ambiguous` alert is replaced by an INFO log naming the
  attention item. The attention item now carries the signal, so the alert log stops filling.
- New API to list, read, resolve and dismiss attention items.
- The Resolve Ambiguous Objects admin page (System Admin → Database Maintenance) lists open
  `ambiguous-object-name` attention items instead of `kb.artifact_objects` rows with
  `reconcile_status = 'ambiguous'`, and gains Resolve and Dismiss actions. Its existing tools
  (use candidate, merge nodes, find linked and rebind, create node) stay.
- **BREAKING (internal):** `GET /api/v1/kb/objects/ambiguous` (the page's old list source) is
  removed. Rows already `ambiguous` before this change stay as they are and no longer appear on
  the page; the existing backfill (`POST /kb/objects/resolve-ambiguous`) is unchanged and can
  still link them.

## Capabilities

### New Capabilities
- `attention-items`: the `kb.attention_items` table, its item types, lifecycle and dedupe rules,
  and the list / read / resolve / dismiss API.
- `object-reconciliation-ties`: how object reconciliation handles tied candidates (tie
  detection, LLM first, tie-break pick, attention item) and the Resolve Ambiguous Objects page
  reading attention items.

### Modified Capabilities
None. Tie handling was specified by ADR 2026070701 (decision DR1, "leave unlinked and warn"),
not by an OpenSpec spec. This change supersedes DR1, and the ADR is amended to say so.

## Impact

- **Database:** one goose migration creating `kb.attention_items`. Its primary key is its own
  `BIGSERIAL`; the change allocates no other IDs, so it uses no idmanager sequence.
- **Backend:** new package `server/api/attention/` (store and HTTP handlers);
  `server/api/doc-processing/artifact_objects.go` (`ReconcileOne` tie detection),
  `object_nodes.go` (`reconcileArtifactObjectsWithLLM` final pass), the processors that build an
  `ObjectReconciler` (extract metrics, provisions, inventory items),
  `server/api/kbhandler/` (remove the old list endpoint), `server/api/routes.go`.
- **Frontend:** `web/src/lib/components/home3/resolve-ambiguous-objects-view.svelte` and
  `resolve-ambiguous-objects-client.ts`; new message keys in `web/messages/en.json` and
  `web/messages/zh-cn.json`.
- **Operations:** none beyond the normal deploy. Issue #18 closes once production shows no new
  `object reconciliation ambiguous` warnings.
- **Docs:** amend ADR 2026070701 (DR1 superseded); devdoc for attention items; register
  `server/api/attention/` as a reusable module.
- **Reusable modules:** the catalogue has no match for a typed human-review queue (searched
  "review queue attention issue triage", "ambiguous object reconcile", "admin resolve backlog",
  "incident alert"). The code search found feature-specific backlogs: `kb.keyword_unresolved`
  (`server/api/ontology/keywords/unresolved_store.go`), entity reconciliation's `needs_human`
  decisions, and the category-tree review queue. Each is tied to its own feature and none
  classifies by type, so the change builds a new table. It borrows the keyword backlog's shape
  (tied candidates captured when seen, hit count, first and last seen). Reused as is:
  `pickTieBreakCandidate`, `fetchSortedCandidates`, the page's merge / rebind / create-node
  endpoints.
- **Out of scope:** the bulk backfill and the existing `ambiguous` backlog. Entity-to-object
  matching (`matchEntityToExistingObject`) also skips ties, but it is match-only by design
  (ADR 2026070101) and is left unchanged. Moving other review pages (such as Resolve Metric
  Range Types) onto attention items is a later change.
