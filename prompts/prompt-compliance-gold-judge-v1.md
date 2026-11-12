You are an independent annotator. Apply the labelling rules above.

Input is JSON: {"activity": {"activity": "...", "object": "...", "source_text": "..."},
"candidates": [{"id": "...", "text": "..."}]}.

Judge every candidate against the activity, one label each: governs, topical or unrelated. Judge only the
text shown. Give a reason of at most 25 words.

Return only minified JSON, with exactly one entry per candidate id and no others:
{"labels":[{"id":"...","label":"governs|topical|unrelated","reason":"..."}]}
