# Compliance gold model-panel rules, version 2 (aligned with benchmark rules 2.0.0)

These rules define what the labeller decides. They are frozen: a change is a new version, never an
edit. They apply to documents of any kind, language and domain; do not rely on the contents of any
particular knowledge base.

These prompts support the deferred independent-annotator/adjudicator panel. Both annotators may
use `ambiguous`; adjudication must preserve unresolved evidence gaps. They do not activate panel
execution. The benchmark skill still uses one agent. Labels are a measuring aid, not legal validation.

The gold aims at every passage of the snapshot that governs an activity. The pool is the search
space for that judgement, so it is searched widely and never filtered by relevance; your labels do
the filtering. Version 2.0.0 includes metric candidates and verifies their original normative
context. It requires
a metric-inclusive version 2 snapshot for new pooling; historical runs retain their own rules and
snapshots when augmented. This gold measures candidate relevance, not complete applicability,
quantitative compliance, severity, omission detection or a full expert assessment.

## 1. Activity

An activity is something the document says is done, provided, performed, stored, processed, offered
or required, which a rule from somewhere else could govern. Examples: "logs user actions in a
database table", "the operator shall collect household waste by category", "the device measures
blood pressure".

- **A1. Label what the document describes, not whether it is right.**
- **A2. Statements without "shall" or "must" count.** Design statements, descriptions, table rows
  and requirements the document imposes on others are activities when a reader would ask "which
  rule governs this?". When unsure, label it.
- **A3. One activity per thing done.** Merge neighbouring lines about the same activity; do not
  split one sentence into several activities. Two different things done in one sentence are two
  activities.
- **A4. Exact lines.** Give `line_start` and `line_end` from the line numbers of the chunk listing,
  inclusive. An activity lies inside one chunk; when its text runs over a chunk boundary, use the
  lines of the chunk that holds its main statement.
- **A5. Not activities:** a table of contents, page headers or footers, a list of references, a
  revision history, or boilerplate that describes nothing being done.
- **A6. Names.** `activity` is short English snake_case (`verb_object`); `object` is the thing acted
  on, in the document's language.

## 2. Document-level activity

- **D1.** Also label the document itself, once: what kind of document it is and its subject (the
  product, service or process it concerns), with the lines that establish them. Set
  `document_level` to true. Its lines lie in the first chunk. At most one per document; leave it out
  only when the first chunk does not establish the document's subject.

## 3. Pool queries and cited documents

The pool already searches each activity's source text and its name with its object. Give it more
ways in; none of this changes what is an activity or how a candidate is labelled.

- **Q1. Queries are search text, never evidence.** A passage is not more or less relevant because a
  query of yours found it.
- **Q2. Write two to six `queries` per activity**, short keyword phrases a reader would use to find
  a rule that governs it: the activity's object and action in other words, the field's own terms,
  and the kind of rule that would govern it. When the document is not in English, write at least
  one query in English and one in the document's language. Do not repeat the source text or the
  name with its object.
- **C1. `cited_docs` are the documents the activity's own lines cite**, or that a clause those lines
  point to cites (for example "按 GB/T 19095 的规定", "as required by Regulation (EU) 2016/679"),
  written as the text cites them, at most ten. List only what the text cites, never a document you
  know of. The pool searches within every edition of each that the knowledge base holds.
- **C2. An unresolved citation is a finding.** When the pool reports that the knowledge base does
  not hold a cited document, record it in the audit notes.

## 4. Candidate judgement

A candidate is a passage of another document that the pool search found for an activity. Choose
exactly one label:

- **`governs`**: the passage states a requirement, prohibition, recommendation or permission that
  would apply to the activity if the passage applied at all. Verify the normative scope of the
  underlying rule (including product class and stated
  conditions); do not decide jurisdiction, edition precedence, legal force, or whether the document
  complies. Those final applicability and assessment decisions belong to a later phase.
- **`topical`**: same subject area, but the passage states nothing that would apply to the activity
  (background, commentary, a definition, a different activity of the same field).
- **`unrelated`**: neither.
- **`ambiguous`**: the evidence leaves a material normative-scope or meaning question unresolved, or
  two careful readers could reasonably decide differently. Do not force certainty; the scorer
  ignores it.

Further rules:

- **J1. Judge verifiable source evidence.** A candidate's title, extraction metadata and retrieval
  score do not prove normative meaning. Inspect its original passage and the source context
  necessary to interpret it (scope, table headings, notes, referenced methods and exceptions).
  Record source record IDs and exact line/clause/table locators in the audit. Context must be
  available in the indexed snapshot or a verifiable original source of that snapshot record; do not
  invent context or import a different edition. An unavailable decisive source makes the judgement
  `ambiguous`, with the missing evidence named.
- **J2. Descriptive or explanatory text is `topical`, not `governs`.** A number alone does not
  establish a rule; apply M1–M5 to quantitative material regardless of candidate type.
- **J3. A passage that copies the activity's own source text is `unrelated`.**
- **J4. Judge each activity separately.** The same passage can govern one activity and be topical
  for another.
- **J5. Reasons.** A `governs` or `ambiguous` label needs a reason of at most 30 words naming what in
  the passage applies, or why readers could differ. Other labels may leave the reason empty.
- **J6. Added passages are judged the same way.** Passages added from cited documents, by expansion
  (other passages of a record with a `governs` label) or by augmentation (passages a reviewer run
  reported that the gold never judged) get labels by the same J1–J5 and M1–M5 criteria. Where a
  passage came from
  (`found_by`) is not evidence: a passage the reviewer found is neither more nor less likely to
  govern.

## Metric and requirement evidence

- **M1. Normative origin.** A metric candidate can be `governs` when the original source establishes
  a requirement, prohibition, recommendation or permission for this activity. A measured result,
  worked example, illustrative value, research observation or aspirational target is not automatically
  binding. Judge its demonstrated normative role; descriptive same-topic values are `topical`.
- **M2. Match the property and scope.** Check what is measured, the product/material/process class,
  applicable conditions and exceptions, and any referenced test or sampling method. A limit for a
  different class or property does not govern merely because the units or keywords match. If a
  material distinction cannot be resolved from evidence, label `ambiguous`.
- **M3. Preserve quantitative meaning.** Read units, inequality direction, ranges, tolerances,
  rounding, basis (for example dry mass versus wet mass), and conditions from source evidence.
  Never invent a missing unit, value, method, exception or numeric default. Numeric comparison and
  pass/fail are outside this gold's labels; identifying an applicable requirement does not establish
  a violation.
- **M4. Trace extraction to source.** An extracted metric/provision is a discovery artifact. Verify
  its source lines and relevant headings/notes before trusting its normalized value or meaning.
  Conflicting extraction and original text are resolved from the verifiable original; record the
  discrepancy. Missing or contradictory decisive context is `ambiguous`, not assumed away.
- **M5. One requirement, several artifacts.** A metric, provision and chunk may represent the same
  source requirement. Label every pooled candidate separately, with its own available evidence;
  group equivalent source requirements in the audit reasoning instead of treating duplicates as
  independent corroboration. Do not merge candidate IDs or omit labels. A broad chunk and a narrow
  extraction may receive different labels when their supported meaning differs.

## 5. No grounding expected

- **N1.** Not labelled directly. An activity has no expected grounding when none of its pooled
  candidates is labelled `governs`. A pool can miss a governing record; cited-document search,
  expansion and augmentation reduce that bias, and the scorer reports recall within the pool.

## 6. Audits before saving

- **U1. Activity completeness.** Re-read every chunk against the activity list and add what was
  missed, including declarative statements and table rows. Check each activity's queries (Q2) and
  cited documents (C1). Then pool again with the full list: activities cannot be added after
  pooling.
- **U2. Activity correctness.** Remove items that describe nothing done (A5), merge split items
  (A3) and check every span (A4).
- **U3. Judgement consistency.** Re-check every `governs` label against J1–J3, and compare labels
  of the same passage across similar activities; an inconsistency needs a reason or a correction.
- **U4. Record the audits** in the audit notes: what was added, removed or relabelled, the
  judgement calls a reviewer should look at first, and the unresolved citations (C2).
- **U5. Quantitative evidence audit.** Re-check metric candidates and numerical requirements of
  every type against M1–M5. Record source locators, unavailable context, extraction conflicts and
  groups of duplicate requirements. Report coverage and readiness limits; no `governs` candidate
  means none found within this pool, not proof that no rule or omitted requirement exists.
