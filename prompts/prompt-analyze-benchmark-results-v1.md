You are reviewing the output of a document-processing benchmark run.

Each document below was generated from a synthetic gold fixture (so its full
source text is known exactly) and then run through one or more real,
unmocked LLM-based extraction processors (e.g. `extract_metrics`,
`extract_provisions`, `extract_entity`, `extract_relation`,
`extract_inventory_items`, `extract_semantic_projections`,
`extract_structured_knowledge`, `extract_doc_metadata`,
`generate_scene_blocks`). For each document you are given:

- its id, family, and title
- its full source text (the exact prose the processors saw, reconstructed
  from the gold fixture's clauses)
- the raw structured rows each requested processor actually produced,
  as JSON, keyed by processor name

Only `extract_metrics` has a hand-authored expected-answer key elsewhere in
this benchmark (a separate verdict-matrix scorer); you are not given that key
here and must not assume one. Your job instead is a close-reading comparison:
does each processor's output plausibly and completely reflect what the
source text actually says? You have the real source text, so you can and
should judge this directly — do not merely restate the JSON.

For every document/processor pair in the input, assess:

1. **Coverage** — is there a specific, identifiable requirement, property,
   entity, or relation in the source text with no corresponding row in the
   processor's output? Quote the missed text.
2. **Correctness** — does any row assert something not supported by the
   source text (a fabricated value, a wrong subject, a wrong relation)?
   Quote the row and explain the mismatch.
3. **Faithfulness of wording** — is the row's language a reasonable
   paraphrase/extraction of the source, or does it drift or invent detail?
4. Only flag a genuine problem. A missing row for a truly vague, unverifiable
   statement (no specific measurable/assessable property at all) is not a
   defect — say so explicitly rather than flagging it.

A processor that produced no rows for a document, or an entry noting "not
applicable" for a processor, is expected in some cases (e.g. a one-line
document has little to extract) — do not treat absence of rows alone as a
defect; check it against the actual source text first.

## Output

Return strict JSON: a single object with one field, `report_markdown`,
whose value is a complete Markdown report with this structure:

```markdown
# Benchmark Analysis: <dataset id> / <case id>

## Summary
<2-4 sentences: overall impression across all documents/processors>

## <document id> (<family>) — <title>
### <processor name>
- **Coverage**: ...
- **Correctness**: ...
- **Notes**: ...
(repeat ### per processor that ran for this document)

(repeat ## per document)

## Cross-Cutting Observations
<patterns that recur across multiple documents/processors, if any>
```

Do not include anything outside the JSON object. Do not translate quoted
source text. Be concrete and cite the specific text/rows you are judging;
avoid generic praise or generic complaints.
