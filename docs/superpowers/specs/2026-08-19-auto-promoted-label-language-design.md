# Auto-Promoted Label Language Resolution

## Goal

Auto-promoted ontology labels are usable governed content, equivalent to a
human-approved label for runtime use. Their `auto-promoted` status records how
they entered the vocabulary; it must not cause an otherwise valid label to be
treated as unavailable.

The auto-promotion path must also assign a useful language tag to its labels.
It must not write `und` merely because it did not inspect the label text.

## Scope

This change is limited to labels created by
`AlignmentsStore.EnsureAcceptedOrCreate` for auto-promoted metric-definition
terms. It does not rewrite existing database rows or change the review and
release workflow for manually authored labels.

## Design

Add a small pure, deterministic language resolver in the ontology keywords
package and call it independently for each auto-promoted preferred and
alternate label.

The resolver examines the label text:

- a non-empty label containing one or more Han characters resolves to `zh`,
  including mixed Han/Latin text;
- a non-empty label containing Latin letters and no Han characters resolves
  to `en`;
- blank text, digits/punctuation-only text, and text containing no Han or
  Latin letters resolves to `und`.

`zh` deliberately identifies Chinese without claiming a Simplified or
Traditional script when the resolver does not determine that distinction.
The resolver is script-based, not a statistical natural-language detector:
the `en` result is the project’s deterministic default for a Latin-script
label, not a claim that it has distinguished English from every Latin-script
language. `und` is available only when the text provides no supported script
signal.

Resolving aliases independently ensures that a multilingual alternate label
does not inherit an incorrect tag from its preferred label. The one-current-
preferred-label rule remains per term and resolved language.

## Runtime behavior

The existing alignment guard accepts an ontology term with status
`auto-promoted` alongside a term with status `included_in_release`. The name
resolver must be changed to apply the same usability rule to both terms and
labels: exact governed-label lookup and preferred-name display must include
`auto-promoted` rows as well as `included_in_release` rows. No status
transition changes are required: auto-promoted labels and terms remain usable
immediately, while their status continues to preserve provenance for later
review or release.

## Tests and verification

Tests will be written before production code to establish that:

1. Chinese metric names create `zh` preferred and alternate labels.
2. English metric names create `en` labels.
3. Mixed Han/Latin labels resolve to `zh`; blank, digits/punctuation-only,
   and unsupported-script labels remain `und`.
4. Auto-promoted `zh`, `en`, and `und` labels resolve through the governed
   exact-label path under requested-language filtering, and preferred-name
   display can select an auto-promoted preferred label.
5. The auto-promotion transaction and existing-alignment reuse behavior remain
   unchanged.

The focused Go package tests will run first, followed by the affected
doc-processing tests and the repository's applicable formatter and build
checks.

## Non-goals

- No machine-learning language-identification dependency.
- No migration or automatic repair of existing orphaned labels.
- No claim that auto-promoted content has received human review.
