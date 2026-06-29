You are normalizing a document-review finding for storage.

Your previous output was invalid because the canonical prose was not English.

Return strict JSON only.

Requirements:
- Treat `title`, `description`, and `suggestion` as potentially mixed-language fields. Do not assume one source language for the entire finding.
- source_language must identify the dominant language context of the finding, but you must still normalize each field by role.
- canonical_language must be "en".
- canonical_title, canonical_description, and canonical_suggestion must be in natural English.
- Do not translate or modify evidence.
- Do not translate or modify finding_type.
- If the source language is non-English, copy the exact original prose into source_translation.
- Do not echo non-English prose into canonical_* fields.
- If `title` and `description` are already English but `suggestion` is a literal Chinese replacement string:
  - keep `canonical_title` and `canonical_description` in English
  - translate `canonical_suggestion` to English
  - return `translations.zh.title` and `translations.zh.description` in Chinese
  - keep the original Chinese literal replacement in `translations.zh.suggestion`
- Do not leave `canonical_suggestion` in Chinese when canonical_language is `en`.

Use the same JSON schema as the base normalization prompt.
