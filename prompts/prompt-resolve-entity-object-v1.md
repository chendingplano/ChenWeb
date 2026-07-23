Decide whether one extracted entity should be linked to a canonical object in
`kb.object_nodes`.

You are given:
- `entity`: the entity as extracted from a document (name, aliases, type,
  description, keywords, categories — attributes only, no document text).
- `object_nodes`: the existing canonical object-node candidates this entity
  might refer to. This list may be empty.

Decide one of three outcomes:

- `exclude` — this entity does not add value as a canonical object. Generic
  or abstract entities (broad concepts, standards, documents cited by other
  artifacts, people, or terms that are not a concrete reusable thing) belong
  here. Once excluded, this entity is never reconsidered — only choose
  `exclude` when you are confident it is not merely under-described.
- `associate` — this entity should be linked to a canonical object.
  - If one of the provided `object_nodes` candidates is clearly the same
    real-world thing as this entity, set `selected_object_id` to that
    candidate's `object_id`. Never invent an object id; it MUST come from the
    provided `object_nodes` list.
  - If no candidate matches but this entity is concrete and reusable enough
    to deserve a new canonical object (equipment, system, material,
    organization, or place — something a metric or provision could plausibly
    be measured/regulated against), leave `selected_object_id` empty and set
    `object_type` to the type the new object should carry.
- `uncertain` — you cannot decide from the entity's attributes alone (too
  thin, too generic, or genuinely ambiguous between candidates). This is not
  a failure: the entity will be reconsidered later if new information (a new
  candidate object, or a richer description from a merged duplicate entity)
  appears. Prefer `uncertain` over guessing.

Confidence (REQUIRED — must be numeric and consistent):
- `confidence` MUST be a number between 0 and 1 (for example 0.92).
- Do NOT use qualitative words such as "high", "medium", or "low", and do NOT
  return the confidence as a percentage.
- A low-confidence `exclude` or `associate` is treated the same as
  `uncertain` downstream — do not inflate confidence to force a decision you
  are not sure of. It is always safe to say `uncertain` with a low confidence
  instead.
- Use a high confidence (>= 0.85) only when the identity or exclusion is
  clear from the attributes given.

Return only the JSON object defined by the schema — no prose, no code fences.
