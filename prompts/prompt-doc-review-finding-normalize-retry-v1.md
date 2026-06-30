You are normalizing a document-review finding for storage.

Your previous output was invalid because the canonical prose was not English.

Return strict JSON only.

## Requirements

- canonical_language must be "en".
- canonical_title and canonical_description must be in natural English. Translate them if not.
- Do not translate or modify evidence or finding_type.
- If the source language is non-English, copy the exact original prose into source_translation; otherwise leave source_translation empty.
- canonical_suggestion: copy the suggestion exactly as-is. Do NOT translate or remove any non-English characters — they are document content the user must paste into their document.

## Translation rule for suggestion

A suggestion consists of two kinds of text:
- **Reviewer instruction** — words explaining what to change ("Simplify the opening:", "Break into multiple sentences:", "Delete the clause;", "rephrase as", "replace with"). These are ALWAYS in the reviewer's language (typically English) and MUST be translated to the target language L.
- **Document replacement content** — the actual text the user inserts into their document, usually quoted. This is in the document's language and must be preserved if already in L.

For translations.<L>.suggestion: translate EVERY reviewer instruction word/phrase to L; keep document replacement content unchanged if already in L. The result must contain ZERO English words except language-neutral identifiers (ISO numbers, code tokens, brand names).

Self-check before outputting: scan translations.<L>.suggestion for English words. Any English word that is not a language-neutral identifier is an error — translate it.

Examples (target L = zh):

```
suggestion:                  "Simplify the opening: '根据对象、属性...'"
translations.zh.suggestion:  "简化开头语：'根据对象、属性...'"
```

```
suggestion:                  "Break into multiple sentences: '注：由于分离...'"
translations.zh.suggestion:  "拆分为多个句子：'注：由于分离...'"
```

```
suggestion:                  "Delete the clause; rephrase as '本类别一般基于光学原理...'"
translations.zh.suggestion:  "删除该分句；将其改写为"本类别一般基于光学原理...""
```

Use the same JSON schema as the base normalization prompt. Produce one entry in `translations` for every language listed in `target_languages`.
