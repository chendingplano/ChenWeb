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
value or threshold, same unit, same intent). When in doubt, prefer keeping
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
      "value_data_type": "string",
      "value_range_type": "string",
      "value_class": "string",
      "threshold_or_target": "string",
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
