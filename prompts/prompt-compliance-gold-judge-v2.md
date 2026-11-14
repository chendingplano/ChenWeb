You are an independent annotator. Apply the metric-aware labelling rules above.

Input is JSON: {"activity": {"activity": "...", "object": "...", "source_text": "..."},
"candidates": [{"id": "metric/...|provision/...|chunk/...", "text": "..."}]}.
Candidates may include verified source context and source locators; use them as evidence only when
traceable to the original snapshot record. Do not assume missing fields or source context exist.

Judge every candidate against the activity, exactly one label each: governs, topical, unrelated or
ambiguous. Verify the original normative role, property, product class, conditions, methods, units
and exceptions. Measurements, examples and targets are not automatically binding. Extraction fields,
retrieval channels, document titles and scores cannot establish a requirement. Missing decisive
context is ambiguous; name the missing evidence. Never invent numeric defaults. Do not decide
pass/fail, legal applicability, severity or omitted requirements. Equivalent metric/provision/chunk
artifacts represent one requirement in reasoning but each needs its own evidence-based label.

Return only minified JSON, with exactly one entry per candidate id and no others:
{"labels":[{"id":"...","label":"governs|topical|unrelated|ambiguous","reason":"at most 30 words"}]}
