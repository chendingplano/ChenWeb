## 1. Storage

- [x] 1.1 Goose migration creating the seven `kb.category_*` tables (devdoc §8.1)
- [x] 1.2 Apply it to the dev and test databases; verify with `\d`

## 2. Tree engine (pure)

- [x] 2.1 `tree.go`: node/tree types, children index, subtree and ancestor helpers
- [x] 2.2 `validate.go`: §3.3 checks; unit tests for each check
- [x] 2.3 `changeset.go`: add/delete/move/update, "others" reconciliation, log entries, stale effects; unit tests
- [x] 2.4 Replay of log entries into a tree; unit test against a change sequence

## 3. Store

- [x] 3.1 Create tree (initial change set), load current tree, apply change set in one transaction with stale marking
- [x] 3.2 Revisions, changes listing, view at revision/seq via replay
- [x] 3.3 Tree types, bindings publish/read (direct + ancestors), members (recursive CTE), others report
- [x] 3.4 Classification records, review queue, review actions
- [x] 3.5 Integration test against TEST_DATABASE_URL

## 4. Classification

- [x] 4.1 `classify.go`: verification walk, thresholds, batching, retries, budget; unit tests for the devdoc §7 consistency cases
- [x] 4.2 Prompts `prompt-category-membership-policy-v1.md` and `prompt-category-membership-question-v1.md`
- [x] 4.3 `decider.go`: Jev / jev_emulated decider from `CATEGORY_DECISION_MODEL`; export `docprocessing.ResolveModelsFilePath`
- [x] 4.4 `source.go`: metrics artifact source with proposals and language
- [x] 4.5 Runner: classify record (with cleanup of vanished artifacts), reclassify stale

## 5. API

- [x] 5.1 `handler.go` endpoints of design D9 and route registration
- [x] 5.2 Build server; endpoint test via httptest on the test DB; live decider run (deepseek-flash, jev_emulated)

## 6. Wrap-up

- [x] 6.1 Update the devdoc status/implementation note; go vet + tests
- [x] 6.2 Commit via jj (only this change's files)
