You are normalizing a document-review finding for storage.

Return strict JSON only.

Task:
1. Detect the source language of the natural-language prose fields.
2. Make the canonical stored prose English.
3. Preserve the original source-language prose when the source language is not English.
4. Never translate or modify evidence.
5. Never translate or modify finding_type.
6. When `title` and `description` are already English but `suggestion` is a literal Chinese replacement text, produce both:
   - canonical English `suggestion`
   - a precomputed `translations.zh` entry whose `title` and `description` are Chinese and whose `suggestion` is the original Chinese replacement text

Input JSON:
{
  "canonical_language": "en",
  "finding": {
    "severity": "high | medium | low",
    "finding_type": "grammar | spelling | punctuation | capitalization | ...",
    "title": "...",
    "description": "...",
    "evidence": "...",
    "location": "...",
    "suggestion": "...",
    "confidence": 0.0
  }
}

Rules:
- Detect language primarily from title and description.
- suggestion may be a literal corrected replacement string in the document's original language; do not let that alone force source_language away from English when title and description are clearly English.
- finding_type is a stable machine code, not user-facing prose.
- If the source language is already English, keep the prose as-is.
- If the source language is not English, translate title, description, and suggestion into natural, precise English.
- When the source language is not English, preserve the original source prose in source_translation.
- Preserve meaning exactly. Do not add, delete, soften, or expand the finding.
- If the source language is mixed, pick the dominant language. If truly unclear, use "und".
- source_translation should be empty when source_language is English.
- When title and description are already English but suggestion is a source-language literal replacement text, keep the English title/description and preserve the literal replacement meaning.

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
