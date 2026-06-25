# DR17 — Restart & Crash Recovery for Stalled Doc-Review Runs

A document-review run can be left **stalled** — stuck in status `running` (or
`accepted`) with one or more aspects never reaching a terminal state — when the
backend process is killed mid-run (deploy, crash, `Ctrl-C`). Because findings are
persisted only once a whole run finishes (a single `SaveFindings` at the end of
`ReviewProcessor.PostProcessIndex`), a stalled run has **saved nothing**: no
findings, no `success` aspects. It simply sits in the *Active Reviews* monitor
forever with no worker driving it.

DR17 makes such runs recoverable two ways:

1. **Manual** — a **Restart** button in the *Active Reviews* card.
2. **Automatic** — a one-shot recovery sweep when `doc-processor` starts.

Both re-run the request **from scratch**: every non-finished sub-task is reset
*as if it had never run*, which is correct precisely because partial sub-task
results are never persisted (DR-note below).

## Why a full re-run is correct

`PostProcessIndex` runs all enabled reviewers, collects their findings in memory,
and writes them **once** at the end:

- mid-run crash ⇒ `kb.doc_review_findings` has **nothing** for the run;
- aspect rows in `kb.doc_review_status` are `pending`/`running` (progress is
  updated live, but that is cosmetic — no findings back it);
- a re-run begins with `DeleteFindings` (idempotent re-run, DR7) and recomputes
  everything.

So resetting all aspects to `pending` and re-running loses no committed work.

## Re-arm semantics (`DocReviewController.RestartRequest`)

`server/api/doc-reviews/controller.go`

1. Reject if the request is already `completed` (HTTP 409 — nothing to restart).
2. `UPDATE kb.doc_review_requests SET status='accepted', start_time=NULL,
   end_time=NULL, error_message=NULL`.
3. `UPDATE kb.doc_review_status SET status='pending', progress=0,
   finding_count=0, error_message=NULL, start_time=NULL, end_time=NULL` for the
   whole `review_run_id`.

After re-arming, the run is re-triggered exactly like a fresh submit:
`PublishReviewEvent` (JetStream), falling back to an inline goroutine.

Duplicate runs are prevented by `RunReview`'s atomic claim
(`UPDATE … WHERE id=$1 AND status='accepted'` + `RowsAffected` check): if a
re-armed request is triggered by both the recovery sweep and a redelivered
JetStream event, only one transitions `accepted → running`; the other logs
*"claimed by another worker; skipping"* and returns.

## Manual Restart (GUI)

| Layer    | Location                                                                  |
| -------- | ------------------------------------------------------------------------- |
| Route    | `POST /api/v1/doc-review/requests/:id/restart` (`server/api/routes.go`)   |
| Handler  | `docreviews.RestartRequest` (`server/api/doc-reviews/handler.go`)         |
| Service  | `restartRequest(id)` (`web/src/lib/services/docReviewService.ts`)         |
| UI       | **Restart** button in `web/src/lib/components/home3/doc-review-monitor.svelte` |

The button sits beside **Stop**/**View** on each active job, shows a
`Restarting…` busy state, and re-polls the monitor on success.

## Automatic recovery on startup

`DocReviewController.RecoverStalledReviews(ctx)` selects every request in
`('accepted','running')`, re-arms each via `RestartRequest`, and returns the
re-armed ids. `cmd/doc-processor/main.go` calls it once at boot and launches
`RunReviewAndReport` for each recovered id.

- **Disable** with `DOC_REVIEW_RECOVER_ON_START=false`.
- **Multi-instance caveat:** the sweep assumes a single `doc-processor`. With
  multiple instances, a restart of one could re-arm a request another is
  actively running. Set `DOC_REVIEW_RECOVER_ON_START=false` on all but one
  instance in that topology.

### Before DR17

```
doc-review event received                          request_id=26
review request already handled; skipping duplicate run   request_id=26 status=running
```

The redelivered JetStream event was skipped and the run stayed stalled. After
DR17 the startup sweep re-arms `#26` to `accepted` and re-runs it (the
redelivered event then either helps run it or is deduped by the atomic claim).

## Knowledge changed / docs touched

- **New code:** `RestartRequest`, `RecoverStalledReviews` (controller),
  `RestartRequest` (handler), `/restart` route, `restartRequest` (service),
  Restart button (monitor), startup sweep (`doc-processor`).
- **Docs updated:** this file (sits alongside `docs/doc-review-correction-report.md`;
  `docs/doc-index.md` indexes KnowledgeStore docs only, so no entry there).
- **Stale docs:** none.
- **Left undocumented (pre-existing):** per-request aspect selection is not
  filtered into the processor (all *config-enabled* reviewers run regardless of
  the request's selected aspects) — out of scope for DR17.
