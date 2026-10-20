# Code Monitor Design

## Goal

Maintain a bilingual System Admin page at `development → System Admin → System → Code Monitor` that shows daily project line totals and frontend/backend line changes grouped by configured projects.

## Configuration and collection

`frontend.projects-to-monitor` contains workspace-relative project directory names that also serve as the group keys shown on the page. For each project, the collector scans all Git-tracked files with recognized source extensions. It classifies files under `web/src`, `svelte/src`, `frontend/src`, or `client/src` as frontend and other recognized project files as backend.

At 2:00 am in the configured workspace timezone, a wall-clock daily job collects one snapshot per project and local calendar date. If `kb.code_monitor` is empty on startup, collect an initial snapshot using today's local date and a synthetic collection timestamp of 2:00 am. The collector counts physical lines across recognized source files: blank lines count, a final line without a newline counts, and an empty file has zero lines. The persistent table stores total project lines and absolute frontend/backend line totals. The API calculates component deltas by subtracting the preceding recorded snapshot; the first snapshot has zero deltas. A uniqueness constraint on project and collection date makes retries idempotent.

## API and page

Add a read endpoint that accepts a time range and returns daily rows grouped by project. Each row includes project, frontend, and backend line totals and their deltas from the previous recorded snapshot; the first snapshot's deltas are zero. The page renders three ECharts bar charts per project on one row: project total, frontend, and backend. Each chart has paired total/delta bars on separate value axes. Date boundaries use the configured workspace timezone: Today is the current local date; Yesterday is the prior local date; Last 7 days is today plus the prior six dates; Last month is the trailing 30 local dates including today; Last 6 months and Last 12 months are rolling calendar windows ending today and beginning six or twelve months earlier. If the target month has fewer days than today's date (for example, March 31 minus one month), clamp the start to the last day of that target month. Both ends of each range are inclusive. The page and navigation label use Paraglide messages in English and Chinese. An empty configured project list gets a localized empty state.

## Navigation and access

Add `Code Monitor` as a child under the existing `sysadmin-system` group. Register its route in the development navigation using the project's existing route conventions. Page visibility and authorization continue to follow the current home3 page and API authorization patterns.

## Failure behavior

Collection logs filesystem, Git, and database failures through the app logger. A failed project collection does not prevent other configured projects from being collected. The daily uniqueness constraint prevents a retried job from creating duplicate snapshots. The API returns only stored snapshots in the selected date range; gaps remain gaps rather than being interpolated.

## Scope notes

- The recognized extension set is an implementation detail and must be documented next to the classifier.
- New page text must be added to both Paraglide message files.
- Use a Goose project migration for `kb.code_monitor`.
- Leave unrelated existing edits in `server/api/kbhandler/metrics_handler.go` and its tests untouched.

## Success criteria

- Configured repository-relative project paths produce one daily snapshot per project.
- Collection is scheduled at 2:00 am in the workspace timezone and does not drift with daylight-saving changes.
- The table has a unique project/date key and can be migrated with Goose.
- The API returns range-filtered daily snapshots grouped by package.
- The development System Admin navigation opens the Code Monitor page.
- The page shows total lines and frontend/backend deltas for each configured project with all six requested date ranges and English/Chinese text.
