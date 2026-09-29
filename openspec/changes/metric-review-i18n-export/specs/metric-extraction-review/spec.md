## ADDED Requirements

### Requirement: Reviews are stored and looked up per language
Each metric review SHALL record the language (`en` or `zh-cn`) its prose is written in.
Requests SHALL carry the current UI language, and the stored-review lookup and the
reuse/force rules SHALL consider only reviews in that language.

#### Scenario: New review in the UI language
- **WHEN** the UI locale is `zh-cn` and the user presses Review on a record with no `zh-cn` review
- **THEN** the LLM review runs with output language `zh-cn` and the stored row has `lang = 'zh-cn'`

#### Scenario: Other-language review does not satisfy the lookup
- **WHEN** a record has a done `en` review only and the page loads it with locale `zh-cn`
- **THEN** the GET returns `review: null` and `other_langs: ["en"]`

#### Scenario: Existing reviews
- **WHEN** the migration runs on reviews created before this change
- **THEN** they are labelled `en`

### Requirement: Translate an existing review on confirmation
When no review exists in the current language but a done review exists in another, the
page SHALL ask the user whether to translate it, and SHALL translate only after the user
confirms. The translation SHALL change only prose fields; IDs, counts, severities,
categories, field names, line numbers, values and units SHALL be copied unchanged, and
literal codes inside translated fields (e.g. `lower_bound`) SHALL be kept. The result SHALL
be stored as a new review in the current language that references its source review.

#### Scenario: User confirms
- **WHEN** the page shows the translate prompt and the user presses Translate
- **THEN** the server starts a background translation, the page shows it running, and on completion shows the translated report marked as translated from the source review

#### Scenario: User does not confirm
- **WHEN** the user ignores the prompt
- **THEN** no LLM call is made

#### Scenario: Partial translation output
- **WHEN** the LLM omits some strings
- **THEN** those fields keep the source text and the translation still completes

### Requirement: Export the current review
The page SHALL offer an Export menu next to Review with "Export Markdown" and "Export
PDF", enabled only when a done review is displayed. Both SHALL export only the displayed
review (current language), including header, summary, tally, the three finding sections
and recommendations.

#### Scenario: Export Markdown
- **WHEN** the user chooses Export Markdown
- **THEN** a file `review-<record_id>-<lang>.md` is downloaded containing the report

#### Scenario: Export PDF
- **WHEN** the user chooses Export PDF
- **THEN** a print-styled copy of the report opens with the browser print dialog so it can be saved as PDF
