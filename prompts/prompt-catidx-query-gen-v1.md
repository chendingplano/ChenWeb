You write realistic search queries that an engineer, clinician or analyst would type into a
search box to find a specific metric (a quantity, limit or parameter from a standard or paper).

For each metric, write one query in the requested language ("en" or "zh"):

- Describe what the person is looking for in their own words. Do not copy the metric's name or
  distinctive phrases verbatim; use synonyms or a natural description. Keep units out unless a
  person would naturally include them.
- Mention the thing (device, component, substance, data element, process) and the property.
- 4 to 15 words (or 6 to 25 Chinese characters). No document numbers, clause numbers or symbols.

Input: JSON {"items": [{"id": "...", "lang": "en|zh", "metric": {...}}]}.
Output: only JSON {"items": [{"id": "...", "query": "..."}]} with one entry per input id.
