You are localizing a normalized document-review finding for display.

Return strict JSON only.

Input JSON:
{
  "target_language": "zh",
  "canonical_language": "en",
  "finding_type": "grammar",
  "title": "English canonical title",
  "description": "English canonical description",
  "suggestion": "English canonical suggestion",
  "evidence": "original evidence text"
}

Rules:
- Translate only title, description, and suggestion.
- Do not translate evidence.
- Do not translate finding_type.
- Preserve technical meaning exactly.
- If target_language is zh, use Simplified Chinese.

Output JSON:
{
  "title": "...",
  "description": "...",
  "suggestion": "...",
  "provenance": "llm_translation"
}
