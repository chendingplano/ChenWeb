# Code Monitor

Code Monitor tracks daily source size for each configured project path. The development workspace groups three charts under each project: total lines, frontend files, and backend files. Each chart shows both the total and its change from the previous recorded day.

## Configuration and collection

In `config.local.toml`, set project directory names relative to the workspace root under `[frontend]`:

```toml
projects-to-monitor = ["ChenWeb"]
```

For each project, the collector uses Git-tracked files under `web/src` for frontend metrics and under `server` for backend metrics. It counts physical lines in recognized source files, including blank lines and a last line without a newline; an empty file has zero lines. Recognized extensions are `.js`, `.jsx`, `.ts`, `.tsx`, `.svelte`, `.css`, `.scss`, `.html`, `.vue`, `.json`, `.go`, `.py`, `.rs`, `.java`, `.kt`, `.php`, `.rb`, `.sql`, `.sh`, `.yaml`, `.yml`, and `.toml`. Other extensions and files outside those two directories are ignored.

One snapshot per project is collected at 2:00 am in the configured workspace timezone. If the table is empty during startup, the server immediately records an initial snapshot for the current local date with `collected_at` set to that date's 2:00 am. Later collections run daily at 2:00 am. In a timezone that skips 2:00 am for the spring daylight-saving transition, that day's scheduled collection runs at the first valid time after the missing hour. Project/date uniqueness makes each daily write idempotent.

Snapshots are stored in `kb.code_monitor`. The authenticated read endpoint is `GET /api/v1/code-monitor?range=<range>` and returns daily rows grouped by project. Each metric's delta is the current total minus the preceding recorded snapshot for that project; the first snapshot has a delta of zero. Supported ranges are `today`, `yesterday`, `last_7_days` (today and six prior days), `last_month` (30 dates ending today), `last_6_months`, and `last_12_months`. The six- and twelve-month ranges use rolling calendar boundaries ending today and clamp the start day to the last day of its target month when necessary.

## Page

Open **Development → System Admin → System → Code Monitor**. Each project has three ECharts bar charts. Each chart plots Total against the left y-axis and Delta against the right y-axis. The page supports English and Chinese through Paraglide.
