## ADDED Requirements

### Requirement: Extraction prerequisite
Before reviewing, the reviewer SHALL check that `kb.doc_process_runs` holds a successful run listing
`extract_metrics_simple` and a successful run listing `extract_provisions` for the reviewed record. If
either is missing, the reviewer SHALL stop with an error that names the missing processor, and SHALL
NOT start any extraction.

#### Scenario: Provision extraction never ran
- **WHEN** the record has a successful `extract_metrics_simple` run but no successful `extract_provisions` run
- **THEN** the compliance aspect fails with an error naming `extract_provisions` and no finding is written

#### Scenario: Only full metric extraction ran
- **WHEN** the record's metrics come only from `extract_metrics`
- **THEN** the reviewer stops with an error naming `extract_metrics_simple`

### Requirement: Configuration is complete before review
The reviewer SHALL stop before reviewing when any of `prompt`, `doc_activity_prompt`,
`relevance_prompt`, `judgement_prompt`, `verifier_prompt`, `correction_prompt` is unset or names a
missing file, or when `DOC_REVIEWER_ERROR_CORRECTION_MODEL_NAME` or `DOC_REVIEWER_FALLBACK_MODEL_NAME`
is unset or unresolvable. The error SHALL name the setting or variable. The reviewer SHALL NOT choose a
model or prompt of its own. Loading `doc-review.local.toml` SHALL fail when `effort` in
`[reviewers.compliance]` is set to anything other than `normal` or `high`.

#### Scenario: Missing fallback model variable
- **WHEN** `DOC_REVIEWER_FALLBACK_MODEL_NAME` is unset
- **THEN** every chunk task fails with an error naming `DOC_REVIEWER_FALLBACK_MODEL_NAME`

#### Scenario: Unknown effort
- **WHEN** `[reviewers.compliance]` sets `effort = "max"`
- **THEN** loading the configuration fails with an error naming `effort` and the values `normal` and `high`

### Requirement: Metric and provision activities
Every metric and provision of the reviewed record SHALL become an activity of the chunk in which its
first line falls. Exact copies (same metric name, value, unit and condition; same provision text)
SHALL merge into one activity that lists every location. After merging, the reviewer SHALL check at
most `max_metrics` metric activities and `max_provisions` provision activities (default 100 each),
choosing the first in line order. Activities beyond the limits or without lines SHALL be counted in
one `compliance_unchecked` finding written with the first chunk.

#### Scenario: More metrics than the limit
- **WHEN** a record has 150 distinct metrics and `max_metrics` is 100
- **THEN** the first 100 metrics in line order are reviewed and the `compliance_unchecked` finding reports 50 unchecked metrics

#### Scenario: Two identical metrics
- **WHEN** two metrics share name, value, unit and condition
- **THEN** one activity is reviewed and its stored activity lists both locations

### Requirement: Activity groups
Metric and provision activities of a chunk whose line ranges overlap SHALL form one activity group. A
group SHALL be judged together and written as one finding; each member SHALL keep its own search,
relevance decisions and verdicts.

#### Scenario: Provision and metric on the same line
- **WHEN** a provision and a metric both come from line 726
- **THEN** one finding covers both, with verdict lines for each member's candidates

### Requirement: Text activities skip what metrics and provisions state
The detect call SHALL receive the chunk's metric and provision activities as `existing_activities`.
The reviewer SHALL drop a returned text activity whose lines all lie inside the lines of the chunk's
metric and provision activities. The document-level activity SHALL still be detected from the first
chunk.

#### Scenario: Activity inside a provision's lines
- **WHEN** the model returns a text activity on lines 10–11 and a provision covers lines 10–12
- **THEN** the text activity is dropped

### Requirement: Derived activity evidence uses supplied text
When a text activity omits `source_text`, the reviewer SHALL derive its quote from supplied rows
inside the requested range and normalize its endpoints to the first and last supplied rows in that
range. Image rows omitted from serialization SHALL NOT cause rejection of otherwise grounded text.
The reviewer SHALL reject invalid or out-of-passage ranges, ranges containing no supplied text, and
explicit quotes whose endpoints are absent or whose text does not match.

#### Scenario: Activity ends on an omitted image
- **WHEN** an activity cites lines 449–453 without `source_text`, lines 449–452 are supplied, and line 453 is an omitted image within the passage
- **THEN** the reviewer derives the quote from lines 449–452 and stores that range without a correction call

#### Scenario: Activity extends outside the passage
- **WHEN** an activity's requested range extends outside the passage even though some of its lines are supplied
- **THEN** the reviewer rejects the activity and asks for a range within the passage

### Requirement: Search per activity kind
A metric activity SHALL search metrics only, by reference to its indexed artifact, keeping at most
`top_metrics` results; a provision activity SHALL search provisions only, keeping at most
`top_provisions`; a text activity SHALL search provisions, metrics and document passages together,
keeping at most `top_artifacts` distinct results across its queries (defaults 30). Every search SHALL
exclude the reviewed record and records with the same md5. At most `max_cited_documents` cited
documents (default 0) SHALL be searched. The names of the activity's cited documents SHALL be saved in
`kb.doc_review_findings.cited_documents`. For each search the finding SHALL store the query text or
reference, keywords, kinds, limit, excluded records, a possibly-cut-off flag and the results.

#### Scenario: Metric activity search
- **WHEN** a metric activity is searched
- **THEN** the search request uses the metric's search-index reference, the metric kind only, and limit `top_metrics`

#### Scenario: Cited documents not searched by default
- **WHEN** a text activity cites "GB/T 1234" and `max_cited_documents` is 0
- **THEN** no search runs for the citation and `cited_documents` holds "GB/T 1234"

### Requirement: Relevance check
After searching, the reviewer SHALL send one structured call per activity with `relevance_prompt`, the
activity and all its candidates, each under a key `c1` to `cN`. For every candidate the answer SHALL
give `relevant` (`yes` or `no`) and `score` (a whole number from 0 to 9). The call SHALL run at
temperature 0 and SHALL get the three tries of every structured call. An answer that leaves out a
candidate, names a key that is not a candidate, or gives an invalid value SHALL be a wrong answer, and
the correction request SHALL name each problem.

A candidate SHALL be kept when its score is at least the minimum that `effort` in
`[reviewers.compliance]` sets: 3 for `normal` (also when unset) and 2 for `high`. Removed candidates
SHALL get no verdict and SHALL stay in the stored relevance decisions with their answer and score. When
no candidate remains, the finding SHALL be `compliance_no_grounding`.

#### Scenario: One call per activity
- **WHEN** an activity has 30 candidates
- **THEN** the reviewer sends one relevance call listing all 30, not one call per candidate

#### Scenario: Unrelated candidate removed
- **WHEN** the model gives a candidate score 1 and the effort is `normal`
- **THEN** the candidate gets no verdict, is absent from the finding's candidate list, and appears in the stored relevance decisions with score 1

#### Scenario: High effort keeps a weaker candidate
- **WHEN** the model gives a candidate score 2
- **THEN** it is removed when the effort is `normal` and judged when the effort is `high`

### Requirement: Judgement call
For each text activity and each activity group, the reviewer SHALL send structured calls with
`judgement_prompt` containing the activity's chunk, the activities and the remaining candidates (text
cut to 2,000 characters), each call holding at most 20 activity–candidate pairs. A failed call SHALL
NOT discard the verdicts of the unit's other calls. Every activity-candidate pair SHALL get exactly one verdict (`compliant`,
`compatible`, `not_compliant`, `undetermined`, `unqualified`) and a summary; a binding (`mandatory`,
`recommendation`, `none`) MAY be left out except for compliant and not compliant verdicts, and a
missing binding SHALL count as `none`. A compliant or not compliant verdict SHALL have a mandatory
binding, a candidate quote that occurs in the candidate text and a document quote that matches the
cited chunk lines. A recommendation SHALL never be compliant or not compliant. An answer that breaks
these rules SHALL be a wrong answer.

#### Scenario: Large group judged in several calls
- **WHEN** a group has three metrics with 15 kept candidates each
- **THEN** three judgement calls run and the one finding holds all 45 verdicts

#### Scenario: Compliant verdict without a document quote
- **WHEN** the model returns `compliant` with an empty `document_quote`
- **THEN** the answer is wrong and the correction request names the activity, the candidate and the field

### Requirement: Numeric comparison
The quantity-comparison module SHALL decide a verdict that carries a numeric comparison, has a
mandatory binding and both quotes, and is compliant, compatible or not compliant:
supported gives compliant, contradicted gives not compliant, any other result gives undetermined with
the module's reason as note.

#### Scenario: Stricter documented limit
- **WHEN** the candidate requires at most 2 kW and the documented limit is at most 1,800 W
- **THEN** the verdict is compliant

### Requirement: Verification of strong verdicts
The reviewer SHALL verify every compliant and not compliant verdict with one structured call using
`verifier_prompt`, only for units that have such verdicts. A rejected verdict SHALL become undetermined
with the verifier's reason as note. A finding SHALL be medium severity only when a verified not
compliant verdict remains; otherwise informational.

#### Scenario: Verifier rejects a not compliant verdict
- **WHEN** the verifier returns `valid: false` for a not compliant verdict
- **THEN** the verdict becomes undetermined with the reason as note and the finding is informational

### Requirement: Three tries per structured call
Activity detection, relevance, judgement and verification calls SHALL each get up to three tries: the reviewer's
model; then, after a wrong answer, a correction request (original input plus `correction_prompt` with
the errors and the previous answer) to `DOC_REVIEWER_ERROR_CORRECTION_MODEL_NAME`; then a correction
request using the latest wrong answer and its errors to `DOC_REVIEWER_FALLBACK_MODEL_NAME`. A try after a provider failure with no wrong
answer yet SHALL send the original input. The chain SHALL NOT raise warnings. Every failed try SHALL be
written to `kb.failed_llm_calls` with the record, review run, aspect, prompt, attempt number, model,
error, raw answer and whether a later try succeeded.

The correction model SHALL run with `thinking.type = "disabled"`, overriding its model profile's
thinking setting, including when retrying after a provider failure. The primary and fallback
models SHALL retain their existing thinking settings.

Each actual request SHALL produce one usage event with its input, response and outcome. Each retry
SHALL link the immediately preceding failed event using `metadata_json.correcting_call_id` and the
note `correcting call: <event-id>` in `error_message`. A successful retry SHALL contain only that note;
a failed retry SHALL retain its error followed by the note. A note-only message SHALL NOT count as a
failure in the usage log or daily summary. Event IDs SHALL come from the individual invocation,
not a shared client's last-usage state. If capture did not persist an event, no ID SHALL be invented.

#### Scenario: Second request succeeds
- **WHEN** C1 fails validation and C2 succeeds
- **THEN** C1 retains its rejected response and error, and C2 records `correcting call: <C1-event-id>` without being counted as a failure

#### Scenario: Third request follows another wrong answer
- **WHEN** C1 and C2 fail validation
- **THEN** C3 corrects C2's answer and records `correcting call: <C2-event-id>`; a rejected C3 is recorded as failed

#### Scenario: Correction model fixes the answer
- **WHEN** the reviewer's model returns a wrong answer and the correction model returns a valid one
- **THEN** the valid answer is used and one `kb.failed_llm_calls` row with attempt 1 and outcome `recovered` is written

#### Scenario: Correction profile enables reasoning
- **WHEN** the correction model's profile enables thinking and the primary request fails
- **THEN** the second request sends `thinking.type = "disabled"`, while primary and fallback thinking settings are unchanged

#### Scenario: All three tries fail
- **WHEN** all three tries return wrong answers
- **THEN** three rows with outcome `failed` are written and the activity's finding is `compliance_incomplete` with `error_msg` set

### Requirement: Errors do not stop the reviewer
The reviewer SHALL write a failure while reviewing one activity or group (search, relevance check,
judgement after three tries, verification) to the finding's `error_msg`, and SHALL continue with the
next activity. A detection failure SHALL produce one `compliance_incomplete` finding for the
chunk's lines. When the run ends, every compliance finding SHALL get `proc_status` `failed` if any of
them has an error, else `success`, and the compliance aspect status SHALL be failed with the error
messages when any error occurred. Other aspects SHALL NOT be marked failed by it.

#### Scenario: One activity fails
- **WHEN** the search fails for one of five activities
- **THEN** five findings are saved, one with `error_msg`, all with `proc_status` `failed`, and only the compliance aspect is failed

### Requirement: Scorer refuses failed runs
`compliance-score` SHALL refuse a reviewer run whose compliance findings have `proc_status` `failed`,
and SHALL skip `compliance_unchecked` findings.

#### Scenario: Scoring a failed run
- **WHEN** a run with a failed compliance finding is scored
- **THEN** the scorer stops with an error that says the run is marked failed

### Requirement: Findings in the report language, never translated
The reviewer SHALL send the first `DOC_REVIEW_REPORT_LANGUAGE` entry (`en` when unset) as
`output_language` in every judgement and verification request, and those prompts SHALL write
summaries and reasons in it. A `compliance_assessment` finding SHALL declare that language. Finding
translation SHALL NOT run for compliance findings: not at save time in either
`DOC_REVIEW_TRANSLATION` mode, not when findings are requested in another language, and not when one
finding is translated on request; the stored finding SHALL be returned as is.

#### Scenario: Chinese report language
- **WHEN** `DOC_REVIEW_REPORT_LANGUAGE` is `zh` and a unit is judged
- **THEN** the judgement request holds `"output_language":"zh"` and the assessment finding declares `zh`

#### Scenario: Viewing a compliance finding in another language
- **WHEN** a compliance finding with no stored translation for `zh-cn` is requested in `zh-cn`
- **THEN** no translation call is made and the stored finding is returned unchanged
