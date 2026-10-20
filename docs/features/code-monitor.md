# Code Monitor

Code Monitor tracks daily source size for every configured project. For each project, three charts show project, frontend, and backend lines. Each chart has paired bars for its line total and its change from the previous recorded snapshot.

## Configuration and collection

In `config.local.toml`, set workspace-relative project directory names under `[frontend]`:

```toml
projects-to-monitor = ["ChenWeb", "shared", "tax"]
```

For each project, the collector scans Git-tracked files throughout the project. Files under `web/src`, `svelte/src`, `frontend/src`, or `client/src` count as frontend; other recognized files count as backend. The total is the sum of both components. It counts physical lines, including blank lines and a last line without a newline; an empty file has zero lines. Recognized extensions are `.js`, `.jsx`, `.ts`, `.tsx`, `.svelte`, `.css`, `.scss`, `.html`, `.vue`, `.json`, `.go`, `.py`, `.rs`, `.java`, `.kt`, `.php`, `.rb`, `.sql`, `.sh`, `.yaml`, `.yml`, and `.toml`.

One snapshot per project is collected at 2:00 am in the configured workspace timezone. If the table is empty during startup, the server immediately records an initial snapshot for the current local date with `collected_at` set to that date's 2:00 am. Later collections run daily at 2:00 am. In a timezone that skips 2:00 am for the spring daylight-saving transition, that day's scheduled collection runs at the first valid time after the missing hour. Project/date uniqueness makes each daily write idempotent.

Snapshots in `kb.code_monitor` store total project lines and absolute frontend/backend line totals. The authenticated read endpoint, `GET /api/v1/code-monitor?range=<range>`, returns daily rows grouped by project, with the total and signed frontend/backend line deltas. A component delta is its current total minus the preceding recorded snapshot for that project; the first snapshot has zero deltas. Supported ranges are `today`, `yesterday`, `last_7_days` (today and six prior days), `last_month` (30 dates ending today), `last_6_months`, and `last_12_months`. The six- and twelve-month ranges use rolling calendar boundaries ending today and clamp the start day to the last day of its target month when necessary.

## Page

Open **Development → System Admin → System → Code Monitor**. Each project has three ECharts bar charts arranged on the same row: project total, frontend, and backend. Each chart shows the corresponding line total and daily delta, using separate axes so the delta remains visible beside a much larger total. A project's first recorded delta is zero. The page supports English and Chinese through Paraglide.
