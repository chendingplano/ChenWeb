You are normalizing a document-review finding for storage.

Your previous output was invalid because the canonical prose was not English.

Return strict JSON only.

Requirements:
- Treat `title`, `description`, and `suggestion` as potentially mixed-language fields. Do not assume one source language for the entire finding.
- source_language must identify the dominant language context of the finding, but you must still normalize each field by role.
- canonical_language must be "en".
- canonical_title and canonical_description must be in natural English.
- Do not translate or modify evidence.
- Do not translate or modify finding_type.
- If the source language is non-English, copy the exact original prose into source_translation.
- For canonical_suggestion: preserve non-English document content verbatim. Do NOT translate replacement text or alternatives written in the source document's language to English — that text is what the user must insert into their document. Only translate the reviewer's instruction language (e.g., "Delete the clause; rephrase as") if needed, but leave the non-English content as-is.
- For each language L in `target_languages`, ALWAYS produce translations.<L>.suggestion as a complete sentence in L: translate any English instruction fragments ("Delete the clause", "finish the sentence after", "or rephrase as") to L, and keep embedded document content unchanged if it is already in L; if the embedded content is in a different language, translate it to L.
- EXCEPTION: if `suggestion` must remain in its original language because the correction itself is a term, standard name, or heading that should stay as-is, keep translations.<L>.suggestion in that original form as well.

Use the same JSON schema as the base normalization prompt. Produce one entry in "translations" for every language listed in `target_languages`.
