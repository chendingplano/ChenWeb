# Localized Document Review Report Design

## Goal

Generate fully Chinese `report-cn` documents while retaining the existing fully English `report-en` documents.

## Decision

Keep one Typst layout and pass it a language-specific label dictionary. This avoids duplicated templates while allowing the shared visual structure to evolve once. The report generator supplies the same lexicon to both the template and its own generated package names, aspect names, assessment text, and fallback messages.

`zh`, `zh-cn`, and `zh-hans` use the simplified-Chinese lexicon and `PingFang SC`; all other currently-supported report languages use English as the safe fallback.

## Data Flow

1. `buildTypstSource` normalizes the report variant language and selects a `reportLexicon`.
2. The generated `.typ` source passes `lang`, `font`, and a `labels` dictionary to `document-review-report`.
3. The Typst template uses `labels` for all static report chrome and sets the appropriate text language/font.
4. Go uses the same lexicon for package/aspect headings and generated assessment, problem, guideline, scope, and reviewer fallback text.
5. Finding title, description, and suggestion remain sourced from the existing localized finding metadata.

## Verification

Tests must assert that Chinese Typst source contains Chinese static labels and generated prose, contains no English template labels, and continues to compile with Typst. English output remains covered by existing compilation tests.

## Documentation impact

ADR 2026062203 already requires English and Chinese reports, so it remains accurate. No ADR change is needed; this design records the implementation detail that one parameterized template serves both variants.
