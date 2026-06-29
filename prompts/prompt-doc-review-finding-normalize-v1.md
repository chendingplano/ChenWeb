You are normalizing a document-review finding for storage.

Return strict JSON only.

Task:
1. Detect the source language of the natural-language prose fields.
2. Make the canonical stored prose English.
3. Preserve the original source-language prose when the source language is not English.
4. Never translate or modify evidence.
5. Never translate or modify finding_type.
6. Handle mixed-language fields correctly. In the common case where `title` and `description` are already English but `suggestion` is a literal Chinese replacement text:
   - keep or produce canonical English `title`
   - keep or produce canonical English `description`
   - translate `suggestion` to canonical English
   - also return a precomputed `translations.zh` entry whose `title` and `description` are Chinese and whose `suggestion` is the original Chinese replacement text

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
- Do not assume the entire finding has only one language. Determine the role of each field separately.
- Detect language primarily from `title` and `description`.
- `suggestion` may be a literal corrected replacement string in the document's original language; do not let that alone force source_language away from English when `title` and `description` are clearly English.
- finding_type is a stable machine code, not user-facing prose.
- If the source language is already English, keep the prose as-is.
- If `title`, `description`, and `suggestion` are all non-English, translate all three into natural, precise English for canonical output.
- If `title` and `description` are already English but `suggestion` is Chinese, translate only `suggestion` into English for canonical output.
- When the source language is not English, preserve the original source prose in source_translation.
- Preserve meaning exactly. Do not add, delete, soften, or expand the finding.
- If the source language is mixed, pick the dominant language. If truly unclear, use "und".
- source_translation should be empty when source_language is English.
- When `title` and `description` are already English but `suggestion` is a source-language literal replacement text, keep the English `title`/`description`, translate `suggestion` to canonical English, and preserve the original Chinese `suggestion` in `translations.zh.suggestion`.
- In that mixed case, also return `translations.zh.title` and `translations.zh.description` in natural Chinese, even if the input `title` and `description` were English.

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
