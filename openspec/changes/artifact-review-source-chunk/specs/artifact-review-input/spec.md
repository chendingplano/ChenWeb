## ADDED Requirements

### Requirement: Artifact review calls carry the extraction chunk

Each LLM call of the `metrics`, `provisions`, `inventory_items` and `metrics_completeness` reviewers SHALL carry, as its source passage, the extraction chunk that contains the artifact under review, taken from the record's saved `.chunks` file. The call SHALL NOT carry the 200-line review window.

#### Scenario: Artifact inside one chunk's regular lines
- **WHEN** a provision's spans are `["43"]` and the record's chunk 2 has overlap lines
  22–26 and regular lines 27–50
- **THEN** the call's source passage contains exactly lines 22–50 and `context_truncated`
  is not set

#### Scenario: Artifact in the overlap of the next chunk
- **WHEN** an artifact's spans run from line 50 to line 52, chunk 2's regular lines are
  27–50 and chunk 3 covers lines 50–69 (line 50 is its overlap line)
- **THEN** the call carries chunk 3, the chunk whose range contains lines 50–52

#### Scenario: Spans run past every chunk
- **WHEN** no single chunk contains all lines from the artifact's earliest span start to
  its latest span end
- **THEN** the call carries the chunk whose regular lines contain the earliest span start
  and the payload sets `context_truncated: true`

#### Scenario: No saved chunk file
- **WHEN** the record has no `.chunks` file
- **THEN** the chunks are rebuilt with the configured `CHUNK_SIZE` and
  `CHUNK_OVERLAP_PERCENT`, and a warning naming the record is logged

#### Scenario: No chunk contains the artifact
- **WHEN** the artifact's spans cannot be parsed or its first line is in no chunk
- **THEN** the call carries the artifact payload without a source passage

### Requirement: Compact source passage format

The source passage SHALL be plain text with one text line per document line, in the form `[<line_number>] <content>`, preceded by `Document: <title and number>` when the document context is known. A line SHALL carry a type tag only for headings (`hN`), tables (`table`), table captions (`caption`), equations (`eq`) and types other than `paragraph` and `list-item` (their raw name). Lines whose type ends in `image` SHALL be left out. The passage SHALL NOT contain per-line JSON keys, overlap flags or page numbers.

#### Scenario: Paragraph and heading lines
- **WHEN** line 36 is `heading-1` "3 术语和定义" and line 43 is a paragraph
- **THEN** the passage contains `[36 h1] 3 术语和定义` and `[43] <line 43 text>`

#### Scenario: Table line
- **WHEN** line 121 is a table
- **THEN** the passage contains `[121 table] ` followed by its numbered rows
  (`121#h0: …`, `121#r1: …`), one row per text line

#### Scenario: Image-only line
- **WHEN** line 120 is `table-image` holding an image path
- **THEN** line 120 does not appear in the passage

#### Scenario: Newline inside a paragraph
- **WHEN** a non-table line's content contains a newline
- **THEN** the newline is replaced by a space, so the line stays one text line

### Requirement: Artifact review units are scheduled by chunk

Artifact review units SHALL be grouped by the chunk they carry: one unit per chunk SHALL run first, then, after `LLM_CALL_STAGGER`, the remaining units, bounded by the run's concurrency limit. Units without a source passage SHALL run in the second phase.

#### Scenario: Several provisions share a chunk
- **WHEN** five provisions map to chunk 4 and two to chunk 7
- **THEN** one chunk-4 unit and one chunk-7 unit start first, and the other five start
  after the stagger

### Requirement: Other reviewers keep their input

The per-chunk and per-block reviewers SHALL keep their existing 200-line windows and page blocks in the existing JSON format.

#### Scenario: Per-chunk reviewer in the same run
- **WHEN** `legal_compliance` runs in the same review run as `provisions`
- **THEN** its calls carry the 200-line JSON windows as before
