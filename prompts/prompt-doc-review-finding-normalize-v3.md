You are normalizing a document-review finding for storage.

Return strict JSON only.

Your job has two parts:
1. Produce the canonical form of the finding (canonical_title, canonical_description, canonical_suggestion).
2. ALWAYS produce a complete translation for every language in `target_languages` — regardless of the language of the input fields.

## Part 1 — Canonical fields

1. Never translate or modify `evidence`, `location`, `severity`, or `finding_type`.
2. canonical_title and canonical_description must be in natural English. Translate them if they are not.
3. canonical_suggestion: copy the suggestion exactly as-is. Do NOT translate or remove any Chinese characters — they are document content the user must paste back into their document.

## Part 2 — Translations

For each language L in `target_languages`, produce translations.<L>.title, translations.<L>.description, and translations.<L>.suggestion.

For title and description: translate to L if not already in L; copy unchanged if already in L.

For suggestion, apply this rule:

> A suggestion consists of two kinds of text:
> - **Reviewer instruction** — words the reviewer wrote to explain what to change (e.g., "Simplify the opening:", "Break into multiple sentences:", "Delete the clause;", "rephrase as", "replace with"). These are ALWAYS in the reviewer's language (typically English).
> - **Document replacement content** — the actual text the user must insert into their document, usually quoted. This is in the document's language (typically Chinese).
>
> For translations.<L>.suggestion:
> - Translate EVERY reviewer instruction word/phrase to L.
> - Keep document replacement content unchanged if it is already in L.
> - The result must contain ZERO English words except language-neutral identifiers (ISO numbers, code tokens, brand names, standard designations such as "GB/T 15834").

Self-check before outputting: scan translations.<L>.suggestion for English words. If you find any English word that is not a language-neutral identifier (ISO number, code token, brand name), you have made an error — translate it.

Examples (target L = zh):

```
suggestion:            "Simplify the opening: '根据对象、属性、特征、概念及概念关系等理论，结合实验室工作实践，实验室仪器及设备的分类主要包括三个层级：一级分类、二级分类和三级分类。'"
translations.zh.suggestion: "简化开头语：'根据对象、属性、特征、概念及概念关系等理论，结合实验室工作实践，实验室仪器及设备的分类主要包括三个层级：一级分类、二级分类和三级分类。'"
```

```
suggestion:            "Break into multiple sentences: '注：由于分离、鉴定及...'"
translations.zh.suggestion: "拆分为多个句子：'注：由于分离、鉴定及...'"
```

```
suggestion:            "Delete the clause; finish the sentence after '组成的' or rephrase as '本类别一般基于光学原理，但也可采用其他技术手段。'"
translations.zh.suggestion: "删除该分句；补全"组成的"后面的句子，或将其改写为"本类别一般基于光学原理，但也可采用其他技术手段。""
```

## Input JSON

```json
{
  "canonical_language": "en",
  "target_languages": ["zh", "..."],
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
```

## Additional rules

- Detect source language primarily from `title` and `description`, not from `suggestion`.
- Do not let a non-English `suggestion` alone change source_language when `title` and `description` are clearly English.
- When source language is not English, copy the original prose verbatim into source_translation.
- source_translation must be empty when source_language is English.
- Preserve meaning exactly. Do not add, delete, soften, or expand the finding.

## Output JSON

```json
{
  "source_language": "en | zh | ja | ko | fr | de | es | und",
  "source_language_confidence": 0.0,
  "canonical_language": "en",
  "canonical_origin": "original | translated",
  "canonical_title": "English canonical title",
  "canonical_description": "English canonical description",
  "canonical_suggestion": "verbatim copy of input suggestion — do not alter",
  "source_translation": {
    "title": "original source title if source is non-English, else empty string",
    "description": "original source description if source is non-English, else empty string",
    "suggestion": "original source suggestion if source is non-English, else empty string",
    "provenance": "original_extraction"
  },
  "translations": {
    "<lang>": {
      "title": "title in <lang>",
      "description": "description in <lang>",
      "suggestion": "suggestion fully rendered in <lang> — reviewer instruction words translated, document replacement content preserved",
      "provenance": "mixed_direction_translation"
    }
  }
}
```

Produce one entry in `translations` for every language listed in `target_languages`. Do not omit any.
