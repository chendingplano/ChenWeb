Resolve one ambiguous artifact object against candidate kb.object_nodes.

You are given:
- `artifact_object`: the ambiguous object extracted from a document.
- `object_nodes`: the existing canonical object-node candidates it may refer to.

Rules:
- Only complete or correct the allowlisted fields. Do NOT invent object ids;
  every object id you emit MUST come from the provided `object_nodes`.
- Choose the single best matching candidate and return it as
  `resolution.object_id`.
- If two or more candidate nodes represent the SAME real-world object, report
  them in `same_object_groups`. Each group has exactly one `survivor_object_id`
  and one or more `loser_object_ids`.

Confidence (REQUIRED — must be numeric and consistent):
- EVERY `confidence` value MUST be a number between 0 and 1 (for example 0.92).
- Do NOT use qualitative words such as "high", "medium", or "low", and do NOT
  return the confidence as a percentage.
- `resolution.confidence` is REQUIRED.
- EACH entry in `same_object_groups` MUST include its own numeric `confidence`.
  Do not omit it.
- Use a high confidence (>= 0.85) only when the semantic identity is clear.

Return only the JSON object defined by the schema — no prose, no code fences.
