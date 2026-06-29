You are normalizing a document-review finding for storage.

Return strict JSON only.

Task:
1. Never translate or modify any field other than 'title', 'description' and 'suggestion'.
2. Translate 'title' and 'description' to the target language, even if they contain large portion of the target language content
3. Translate `suggestion` to the target language if it is English, except `suggestion` is already English because the corrected document text itself should remain English, such as an English heading, title, or bilingual-standard English title. In that case:
   - keep canonical English `suggestion` unchanged
   - preserve `translations.<target_lang>.suggestion` in English unchanged

Input JSON:
{
  "canonical_language": "en",
  "target_language": "zh",
  "finding": {
    "severity": "high | medium | low",
    "finding_type": "grammar | spelling | punctuation | capitalization | ...",
    "title": "...",
    "description": "...",
    "evidence": "...",
    "suggestion": "...",
  }
}

Rules:
- If the source language is already English, keep the prose as-is.
- Preserve meaning exactly. Do not add, delete, soften, or expand the finding.
- source_translation should be empty when source_language is English.

Output JSON:
{
  "source_language": "en | zh | ja | ko | fr | de | es | und",
  "source_language_confidence": 0.0,
  "canonical_language": "en",
  "canonical_origin": "original | translated",
  "canonical_title": "English canonical title",
  "canonical_description": "English canonical description",
  "canonical_suggestion": "English canonical suggestion",
  "source_translation": {
    "title": "original source title",
    "description": "original source description",
    "suggestion": "original source suggestion",
    "provenance": "original_extraction"
  },
  "translations": {
    "zh": {
      "title": "Chinese localized title",
      "description": "Chinese localized description",
      "suggestion": "Chinese localized suggestion",
      "provenance": "mixed_direction_translation"
    }
  }
}
