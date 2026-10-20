## 1. Collection and persistence

- [x] Add `kb.code_monitor` migration and project configuration.
- [x] Count tracked source files from each project's `web/src` and `server` directories.
- [x] Schedule daily snapshots at local 2:00 am and seed an empty table with a snapshot stamped at today's 2:00 am.
- [x] Return project-grouped history and compute per-metric deltas from prior snapshots.

## 2. Page and charts

- [x] Add the Code Monitor navigation entry and page.
- [x] Add the six date-range controls and three metric charts per project.
- [x] Render Total and Delta bars with left and right value axes, respectively.
- [x] Localize interface and chart descriptions in English and Chinese.

## 3. Documentation and verification

- [x] Document the behavior in the feature docs and KnowledgeStore devdoc.
- [x] Add collector, configuration, and API tests.
- [x] Run focused Go and frontend checks.
