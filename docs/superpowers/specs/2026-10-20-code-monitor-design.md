# Code Monitor Design

## Goal

Add a bilingual System Admin page at `development → System Admin → System → Code Monitor` that shows daily code size and file-count history grouped by configured projects.

## Configuration and collection

Add `frontend.projects-to-monitor` to the Go frontend configuration. Each value is a workspace-relative project directory name and is also the project group key shown on the page. For each project, the collector counts Git-tracked source files under `web/src` as frontend and under `server` as backend; files elsewhere in the project do not contribute.

At 2:00 am in the configured workspace timezone, a wall-clock daily job collects one snapshot per project and local calendar date. If `kb.code_monitor` is empty on startup, collect an initial snapshot using today's local date and a synthetic collection timestamp of 2:00 am. The collector counts physical lines across recognized frontend and backend source files: blank lines count, a final line without a newline counts, and an empty file has zero lines. Frontend and backend extensions are explicitly classified in the collector; other tracked files are excluded. A uniqueness constraint on project and collection date makes retries idempotent. The persistent table is `kb.code_monitor` and stores the project path, local collection date, total lines, frontend file count, backend file count, and collection timestamp.

## API and page

Add a read endpoint that accepts a time range and returns daily rows grouped by project. Each row includes the daily total and its delta from the previous recorded snapshot for that project; a project's first snapshot has a delta of zero. The page renders three ECharts charts per project: total lines, frontend files, and backend files. Each chart has two bar series, Total and Delta, with the left value axis for Total and the right value axis for Delta. Date boundaries use the configured workspace timezone: Today is the current local date; Yesterday is the prior local date; Last 7 days is today plus the prior six dates; Last month is the trailing 30 local dates including today; Last 6 months and Last 12 months are rolling calendar windows ending today and beginning six or twelve months earlier. If the target month has fewer days than today's date (for example, March 31 minus one month), clamp the start to the last day of that target month. Both ends of each range are inclusive. The page and navigation label use Paraglide messages in English and Chinese. An empty configured project list gets a localized empty state.

## Navigation and access

Add `Code Monitor` as a child under the existing `sysadmin-system` group. Register its route in the development navigation using the project's existing route conventions. Page visibility and authorization continue to follow the current home3 page and API authorization patterns.

## Failure behavior

Collection logs filesystem, Git, and database failures through the app logger. A failed package collection does not prevent other configured packages from being collected. The daily uniqueness constraint prevents a retried job from creating duplicate snapshots. The API returns only stored snapshots in the selected date range; gaps remain gaps rather than being interpolated.

## Scope notes

- The exact frontend/backend extension sets are implementation details and must be documented next to the classifier.
- New page text must be added to both Paraglide message files.
- Use a Goose project migration for `kb.code_monitor`.
- Leave unrelated existing edits in `server/api/kbhandler/metrics_handler.go` and its tests untouched.

## Success criteria

- Configured repository-relative package paths produce one daily snapshot per package.
- Collection is scheduled at 2:00 am in the workspace timezone and does not drift with daylight-saving changes.
- The table has a unique package/date key and can be migrated with Goose.
- The API returns range-filtered daily snapshots grouped by package.
- The development System Admin navigation opens the Code Monitor page.
- The page shows the three requested metrics per package with all six requested date ranges and English/Chinese text.
