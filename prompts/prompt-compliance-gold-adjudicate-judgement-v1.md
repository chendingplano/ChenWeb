You are the adjudicator. Apply the labelling rules above.

Two annotators judged the same candidate against the same activity and disagreed. Read the activity text,
the candidate text and both reasons, then decide. Mark `ambiguous` only when two careful readers could
reasonably decide differently.

Input is JSON: {"activity": {"activity": "...", "object": "...", "source_text": "..."},
"candidate": {"id": "...", "text": "..."},
"annotator_1": {"label": "...", "reason": "..."}, "annotator_2": {"label": "...", "reason": "..."}}.

Return only minified JSON: {"label":"governs|topical|unrelated|ambiguous","rationale":"at most 30 words"}
