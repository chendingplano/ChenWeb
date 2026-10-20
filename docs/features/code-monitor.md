# Code Monitor

Code Monitor tracks the size of configured source directories over time. The development workspace shows daily line totals and frontend/backend file counts, with separate charts for each configured package and metric.

## Configuration and collection

In `config.local.toml`, set the repo-relative directories under `[frontend]`:

```toml
code-packages-to-monitor = ["web/src", "server"]
```

The collector uses Git-tracked files in each directory. It counts physical lines in recognized source files, including blank lines and a last line without a newline; an empty file has zero lines. The current classifier treats `.js`, `.jsx`, `.ts`, `.tsx`, `.svelte`, `.css`, `.scss`, `.html`, and `.vue` as frontend files, and `.go`, `.py`, `.rs`, `.java`, `.kt`, `.php`, `.rb`, `.sql`, `.sh`, `.yaml`, `.yml`, and `.toml` as backend files. Other extensions are ignored.

One snapshot per package is collected at 2:00 am in the configured workspace timezone. In a timezone that skips 2:00 am for the spring daylight-saving transition, that day's collection runs at the first valid time after the missing hour. The first snapshot is written at the next scheduled collection; starting or restarting the server does not collect immediately. Package/date uniqueness makes each daily write idempotent.

Snapshots are stored in `kb.code_monitor`. The authenticated read endpoint is `GET /api/v1/code-monitor?range=<range>` and returns daily rows grouped by package. Supported ranges are `today`, `yesterday`, `last_7_days` (today and six prior days), `last_month` (30 dates ending today), `last_6_months`, and `last_12_months`. The six- and twelve-month ranges use rolling calendar boundaries ending today and clamp the start day to the last day of its target month when necessary.

## Page

Open **Development → System Admin → System → Code Monitor**. Each package has three ECharts mix-line-bar charts: total lines, frontend files, and backend files. The page supports English and Chinese through Paraglide.
