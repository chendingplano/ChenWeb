## Context

Code Monitor is an internal development page. It records daily snapshots of Git-tracked source files and serves historical values to a bilingual Svelte page.

## Decisions

- `frontend.projects-to-monitor` contains project directory names resolved from the workspace root. The local configuration monitors `ChenWeb`.
- Within each project, only `web/src` contributes frontend metrics and `server` contributes backend metrics. Recognized source extensions are counted; other files and directories are ignored.
- One database row is stored per `(project_path, collection_date)`. Daily collection runs at 2:00 am in the configured workspace timezone. If the table is empty at startup, an initial snapshot is recorded for the current local date with a synthetic 2:00 am timestamp.
- API deltas are calculated against the previous recorded snapshot for the same project, including a previous row outside the requested range. The first snapshot has a delta of zero.
- Each project renders three bar charts. Every chart shows Total on the left y-axis and Delta on the right y-axis.
- The API supports `today`, `yesterday`, `last_7_days`, `last_month`, `last_6_months`, and `last_12_months`.

## Data and interfaces

- `kb.code_monitor` stores `project_path`, local collection date, line and file totals, and collection timestamp.
- `GET /api/v1/code-monitor?range=<range>` returns rows grouped by project, with total and delta values for lines and frontend/backend files.
- The page is registered at Development → System Admin → System → Code Monitor. All visible text uses English and Chinese Paraglide messages.

## Risks and constraints

- Source files with extensions outside the explicit allowlist are not included.
- Project names must identify directories beneath the workspace root.
