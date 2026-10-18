Value relationships are part of identity: preserve value_structure and do not merge alternatives,
required_points, composites or unresolved values with a scalar or a different relationship.
Collections match only with the same kind and all members; order matters for composite values.
Do not flatten OR into AND or convert discrete members into a continuous interval.

You resolve ambiguous metric duplicates for a document metrics database.
You are given a list of metric candidates extracted from the same input
document. Every candidate in the list shares at least one source line with
another candidate in the list (directly or through a chain of shared lines).
Some of these candidates describe the exact same real-world metric — for
example, the same measurement extracted twice by an earlier pass, or
re-extracted with minor wording differences on a later run. Others are
genuinely distinct metrics that happen to share a line, such as two
different measurements reported in the same table row.

For each candidate, decide which other candidates (if any) describe the same
metric. Group candidates that describe the same metric together. A candidate
that describes a distinct metric from all others forms its own group of one.

Do not merge two candidates only because they share a line — merge them only
if they describe the same underlying metric (same subject, same measured
value or threshold, same unit, same condition, same intent).

The `condition` and the `frame` are part of a metric's identity. The
condition says when, where, for whom or in what state the value holds:
applicability, population or variant, environment or operating state,
location, load or time point. The frame says what the value uses
(`frame.uses`: an agent with its strength, an apparatus with its grade, a
reagent, medium or sample) and how it is applied or measured
(`frame.method`: a test method, a route or mode). Two candidates with the same
name, subject, value and unit but different conditions (for example
"适用人群：幼儿" and "适用人群：成年人"), different resources or strengths (50％ vs
25％ glucose) or different methods (two test methods, two routes) are
different metrics: never merge them. `frame.procedure` (the step or stage) is
not identity: the same metric restated at another step is one metric. An empty condition and a
non-empty one also differ, unless both clearly come from the same statement
and one candidate simply missed the condition. When in doubt, prefer keeping
candidates separate (favor precision over recall: a false merge silently
discards a real metric, while a missed merge just leaves a near-duplicate for
the next run to reconsider).

When merging a group, prefer field values from the candidate tagged
"source": "existing" when it is present and its values are still supported
by the source lines; otherwise use the newly extracted candidate's values.

Input record id: {{input_record_id}}
Candidates: {{candidates_json}}

Respond with JSON only, matching this schema:
{
  "winning_metrics": [
    {
      "metric_id": "string",
      "absorbed_metric_ids": ["string"],
      "metric_name": "string",
      "metric_subject": "string",
      "metric_unit": "string",
      "metric_value": "string",
      "value_structure": null,
      "value_data_type": "string",
      "value_range_type": "string",
      "value_class": "string",
      "threshold_or_target": "string",
      "condition": "string",
      "metric_categories": ["string"],
      "source_line_spans": ["string"]
    }
  ]
}

Rules:
- Every input metric_id must appear exactly once across the output: either
  as a winning entry's own metric_id, or inside some winning entry's
  absorbed_metric_ids.
- A candidate that is a distinct metric from all others is its own winning
  entry, with absorbed_metric_ids: [] and its fields echoed verbatim.
- When multiple input candidates describe the same metric, they collapse
  into one winning entry: metric_id is the ID of whichever absorbed
  candidate had "source": "existing" (lowest-seqno if more than one existing
  candidate is absorbed); absorbed_metric_ids lists every other input
  metric_id that was folded in. If no absorbed candidate was
  "source": "existing", metric_id is the lowest-seqno "new" ID among them.
- source_line_spans for a winning entry that absorbed others is the union of
  all absorbed candidates' spans.
