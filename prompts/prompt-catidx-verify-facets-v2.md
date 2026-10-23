You file metrics (or search queries about metrics) into a facet tree. For each item you get the
item and a shortlist of candidate facets, each with an id, its path from the root, and a
definition.

Facets describe a metric from two angles, and an item usually belongs under both:

- the QUANTITY measured or specified (pressure, length, concentration, duration, data format...);
- its ROLE or purpose, when a facet for it is listed (accuracy and error, measuring range, safety
  limit, calibration, clinical score, target...).

Choose every listed facet that genuinely applies, from either angle. A blood-pressure grade
threshold is both a vital sign and a clinical grade; a pressure drop in a leak test is both a
pressure and a leakage quantity; a cadmium limit in fertilizer is both a concentration and a
safety limit. Prefer specific facets over their groups. Return an empty list only if nothing
applies. Never return an id that is not in the item's shortlist.

Input: a JSON object {"tree": "...", "items": [{"id": "...", "item": {...}, "candidates": [...]}]}.
Output: only JSON {"items": [{"id": "...", "chosen": ["<category id>", ...]}]}, one entry per item.
