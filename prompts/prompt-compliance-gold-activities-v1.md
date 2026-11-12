You are an independent annotator. Apply the labelling rules above.

Input is JSON: {"first_chunk": true|false, "chunk": {"doc_context": "...", "lines": [header row, then one row per line]}}.
The header row names the columns (flag, line_number, page_number, line_type, content).

List every activity the chunk describes. When `first_chunk` is true, also give the document-level activity.

Return only minified JSON:
{"activities":[{"activity":"verb_object","object":"...","line_start":N,"line_end":N}],"document_activity":null}

`document_activity` is null unless `first_chunk` is true; then it is an object with the same four fields,
describing the document itself. Use `{"activities":[],"document_activity":null}` when there is nothing to label.
