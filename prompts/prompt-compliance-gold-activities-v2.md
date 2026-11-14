You are an independent annotator. Apply the labelling rules above.

Input is JSON: {"first_chunk": true|false, "chunk": {"doc_context": "...", "lines": [header row, then one row per line]}}.
The header row names the columns (flag, line_number, page_number, line_type, content).

List every activity the chunk describes, including quantitative specifications and table requirements.
Do not decide compliance or infer omitted numeric defaults. When `first_chunk` is true, also give the document-level activity.

For each activity and document activity, supply two to six short discovery `queries` (including
English and the source language for non-English documents) and at most ten `cited_docs` actually
cited by its source lines or the clauses they point to. Queries and citations are not evidence.

Return only minified JSON:
{"activities":[{"activity":"verb_object","object":"...","line_start":N,"line_end":N,"queries":["..."],"cited_docs":[]}],"document_activity":null}

`document_activity` is null unless `first_chunk` is true; then it is an object with the same fields,
describing the document itself. Use only verifiable source lines. Use `{"activities":[],"document_activity":null}` when there is nothing to label.
