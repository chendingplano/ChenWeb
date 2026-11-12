# Compliance gold labelling rules, version 1

These rules define what a labeller decides. They are frozen: a change is a new version, never an edit.
They apply to documents of any kind, language and domain; do not rely on the contents of any
particular knowledge base.

## Activity

An activity is something the document says is done, provided, performed, stored, processed, offered or
required, which a rule from somewhere else could govern. Examples: "logs user actions in a database
table", "the operator shall collect household waste by category", "the device measures blood pressure".

- Label what the document describes, not whether it is right.
- Merge neighbouring lines about the same thing into one activity; do not split one sentence into many.
- Give exact line numbers from the `line_number` column of the input table. `line_start` and
  `line_end` are inclusive and lie inside the chunk.
- Do not label a table of contents, page headers or footers, a list of references, a revision history,
  or boilerplate that describes no activity.
- Text that only defines a term, or only states a rule the document imposes on others without
  describing something being done, is still an activity when a reader would ask "which rule governs
  this?". When unsure, label it.
- Name the activity in short English snake_case (`verb_object`), and give the object in the document's language.

## Document-level activity

In the first chunk only, also label the document itself: its type, its subject (the product, service
or process it concerns) and its purpose, with the lines that establish them. Mark it `document_level`.
There is at most one per document.

## Candidate judgement

A candidate is a passage of another document. Judge only the passage shown, not what you know about
other rules. Choose exactly one label:

- `governs`: the passage states a requirement, prohibition, recommendation or permission that would
  apply to the activity if the passage applied at all. Do not judge jurisdiction, edition or whether
  the document complies; applicability is decided in a later phase.
- `topical`: same subject area, but the passage states nothing that would apply to the activity
  (background, commentary, a definition, a different activity of the same field).
- `unrelated`: neither.

Descriptive or explanatory text is `topical`, not `governs`. A passage that is a copy of the activity's own
source text is `unrelated`.

## No grounding expected

Not labelled directly. An activity has no expected grounding when no pooled candidate is finally
labelled `governs`.

## Ambiguity

Only the adjudicator may mark a judgement `ambiguous`, when two careful readers could reasonably
decide differently. Record why. Do not force a decision.
