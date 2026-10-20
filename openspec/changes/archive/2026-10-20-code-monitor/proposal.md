## Why

ChenWeb's development workspace needs a daily view of code size and file-count changes across configured projects. Storing dated snapshots makes trends visible and comparable over time.

## What Changes

- Add a Code Monitor page under Development → System Admin → System.
- Configure monitored project directories through `frontend.projects-to-monitor`.
- Collect daily line and frontend/backend file counts, and show totals and deltas in charts.
- Persist snapshots in `kb.code_monitor` and expose them through a range-filtered API.

## Capabilities

### New Capabilities
- `code-monitor`: Daily source metrics grouped by configured project, persisted and shown in the development workspace.

## Impact

- ChenWeb Go configuration, collector, API routes, and project migrations.
- ChenWeb home3 navigation, Svelte view, ECharts charts, and English/Chinese messages.
