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

Use the same JSON schema as the base normalization prompt.
