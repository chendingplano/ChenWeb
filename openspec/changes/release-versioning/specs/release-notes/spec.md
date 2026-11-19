## ADDED Requirements

### Requirement: Notes for one release or a range
The system SHALL produce release notes for the releases whose version is after `from` and up
to and including `to`, newest first. Each built release SHALL list its version, date, build
number and items grouped as New features, Improvements and Bug fixes. A new major or minor
inside the range SHALL appear as a heading with its description. An empty `from` SHALL mean
from the first release. Notes SHALL be built from stored rows only, without an LLM.

#### Scenario: One release
- **WHEN** notes are requested from `01.02.05` to `01.02.06`
- **THEN** they contain only release `01.02.06` and its items

#### Scenario: Range across a new minor
- **WHEN** notes are requested from `01.02.05` to `01.03.02`, and `01.03.00` was created with description "Release management"
- **THEN** they list `01.03.02` and `01.03.01`, the heading `01.03 — Release management`, then `01.02.06`

#### Scenario: Release with no items
- **WHEN** a release in the range has no items
- **THEN** it is listed with a line saying no changes were recorded

### Requirement: Issue numbers in the notes
An item's issue number SHALL appear after its description as `(#<n>)` for component ChenWeb
and `(shared#<n>)` for component shared. A shared item without an issue SHALL be prefixed
`[shared]`.

#### Scenario: Shared bug fix
- **WHEN** a release has a shared bug fix with issue 26
- **THEN** its line reads `- DeepSeek span repair corrupted quotes (shared#26)`

### Requirement: Text and PDF forms in English and Chinese
Notes SHALL be available as plain text and as PDF, with headings in English or Chinese as
requested. Item text SHALL be shown as entered. The PDF SHALL be made from the same content
as the text form, compiled with `typst`, and SHALL be reused for identical content. Item
text SHALL appear literally in the PDF, never interpreted as markup.

#### Scenario: Chinese PDF
- **WHEN** an admin requests notes as PDF with `lang=zh-cn`
- **THEN** a PDF is returned whose section headings are Chinese and whose items match the text form

#### Scenario: PDF tools missing
- **WHEN** typst is not installed on the machine
- **THEN** the PDF request fails with 503 naming the missing tool, and the text form still works

### Requirement: Notes are viewable on both machines
The Releases page on both machines SHALL offer text and PDF notes for every built release.
The text SHALL be shown on the page; the PDF SHALL open in a new browser tab.

#### Scenario: View notes on production
- **WHEN** an admin on the deploy machine opens release `01.02.06` on the Releases page and clicks PDF
- **THEN** the notes for `01.02.06` open as a PDF in a new tab
