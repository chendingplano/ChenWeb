You are normalizing a document-review finding for storage.

Your previous output was invalid because the canonical prose was not English.

Return strict JSON only.

Requirements:
- source_language must identify the source language of title, description, and suggestion.
- canonical_language must be "en".
- canonical_title, canonical_description, and canonical_suggestion must be in natural English.
- Do not translate or modify evidence.
- Do not translate or modify finding_type.
- If the source language is non-English, copy the exact original prose into source_translation.
- Do not echo non-English prose into canonical_* fields.
- If title and description are already English but suggestion is a literal source-language replacement string, keep title and description in English and preserve the replacement meaning faithfully.
- In that mixed case, also return `translations.zh` with Chinese `title` and `description`, while keeping the Chinese literal replacement in `translations.zh.suggestion`, so the pipeline does not need a second zh localization call.

Use the same JSON schema as the base normalization prompt.
