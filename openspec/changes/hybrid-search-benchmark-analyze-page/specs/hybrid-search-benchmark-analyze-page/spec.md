## ADDED Requirements

### Requirement: Analyze opens a new tab
Each row of the run page's Questions table SHALL have an **Analyze** link that opens a new browser
tab on the development page with the same site header and menu tree, Run Benchmark selected, and
`analyze=<result id>` in the URL. The tab's working area SHALL show the analysis of that result and
its right panel SHALL show metric details. Choosing another page in the menu SHALL remove
`analyze` from the URL and leave the analysis.

#### Scenario: Open an analysis
- **WHEN** an administrator presses Analyze on a question of run 21
- **THEN** a new tab opens on `/development?view=system-admin&child=sysadmin-hybrid-search-benchmark-run&analyze=<result id>`
  showing the header, the menu tree with Run Benchmark selected, and that question's analysis

#### Scenario: Leave the analysis
- **WHEN** the administrator chooses another page in the tab's menu
- **THEN** that page is shown and the URL no longer has `analyze`

### Requirement: Content panel log
The content panel SHALL show the question, its target, hop type, language and ranks, then its log
in search order: the question's own search and, for Medium/High runs, the effort analysis (strategy
and inferred need) and every attempt with its queries, the judge's verdict, reason and matching
metrics. Each query SHALL show its BM25 keywords, any error and its four candidate lists (BM25 per
partition, BM25 merged with merged and partition ranks, semantic with similarity, fused with RRF
score and input ranks), highlighting the target's rows. Every metric ID in the log SHALL be
clickable. When no trace is stored the panel SHALL say so.

#### Scenario: High-effort question
- **WHEN** the analysis of a High-effort question with one attempt of three queries is opened
- **THEN** the log shows the question's search, the effort analysis, the attempt's three queries
  with their lists, and the judge's verdict with its matching metrics

#### Scenario: Earlier run
- **WHEN** the result was stored before tracing existed
- **THEN** the panel shows the question and its ranks and says that no trace was stored

### Requirement: Clicking a metric
Clicking a metric ID SHALL load that metric's document in the PDF panel using the reusable PDF
viewer, move to the page of its first source line, and highlight its source lines (or the table
rows the viewer locates). It SHALL show the metric in the right panel with the Metric Detail
Visualizer (module `20261025-01`), including its grounding lines. The clicked metric's rows SHALL
be marked in the log. A metric that cannot be found SHALL show a message instead.

#### Scenario: Metric in another document
- **WHEN** the administrator clicks a candidate from a different document than the one shown
- **THEN** the PDF panel loads that document at the metric's page with its lines highlighted and
  the right panel shows that metric's details

#### Scenario: Unknown metric
- **WHEN** the clicked metric ID is not in `kb.metrics` any more
- **THEN** the PDF panel says the metric was not found and the right panel shows no details

### Requirement: Trace response includes the result
`GET /api/v1/kb/hybrid-search-benchmark/results/:id/trace` SHALL return the result (as listed in
its run, including the effort trace) and the run's lexical backend together with the queries.

#### Scenario: Trace of a result
- **WHEN** the trace of a stored result is requested
- **THEN** the response has `result` with its question, target and ranks, `lexical_backend`, and
  its `queries`

### Requirement: Analyze page conventions
All text on the Analyze tab SHALL be available in English and Chinese and SHALL be selectable;
the two panels SHALL be resizable and SHALL stack on narrow screens.

#### Scenario: Chinese interface
- **WHEN** the interface language is Chinese
- **THEN** every label on the tab is Chinese and data (questions, labels, queries) is shown as
  stored
