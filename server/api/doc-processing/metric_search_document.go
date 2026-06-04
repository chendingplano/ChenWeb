package docprocessing

import (
	"encoding/json"
	"strings"
)

func buildMetricSearchDocument(metric map[string]any, includeEnglish bool) string {
	parts := []string{
		strings.TrimSpace(asString(metric["metric_name"])),
		"",
		strings.TrimSpace(asString(metric["subject"])),
		"",
		searchDocumentArrayText(metric["keywords"]),
		"",
		strings.TrimSpace(asString(metric["desc"])),
		"",
		strings.TrimSpace(asString(metric["context"])),
		"",
		strings.TrimSpace(asString(metric["value_class"])),
		"",
		strings.TrimSpace(asString(metric["unit"])),
		"",
		strings.TrimSpace(asString(metric["table_name_or_section"])),
		searchDocumentArrayText(metric["category_paths"]),
		"",
	}
	if includeEnglish {
		parts[1] = strings.TrimSpace(asString(metric["metric_name_en"]))
		parts[3] = strings.TrimSpace(asString(metric["subject_en"]))
		parts[5] = searchDocumentArrayText(metric["keywords_en"])
		parts[7] = strings.TrimSpace(asString(metric["desc_en"]))
		parts[9] = strings.TrimSpace(asString(metric["context_en"]))
		parts[11] = strings.TrimSpace(asString(metric["value_class_en"]))
		parts[13] = strings.TrimSpace(asString(metric["unit_en"]))
		parts[16] = searchDocumentArrayText(metric["category_paths_en"])
	}
	return joinUniqueSearchParts(parts...)
}

func joinUniqueSearchParts(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.Join(strings.Fields(part), " "))
		if part == "" {
			continue
		}
		key := strings.ToLower(part)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, part)
	}
	return strings.Join(out, " ")
}

func searchDocumentArrayText(raw any) string {
	switch v := raw.(type) {
	case nil:
		return ""
	case []string:
		return joinUniqueSearchParts(v...)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s := strings.TrimSpace(asString(item)); s != "" {
				parts = append(parts, s)
			}
		}
		return joinUniqueSearchParts(parts...)
	case json.RawMessage:
		return searchDocumentJSONBytesText(v)
	case []byte:
		return searchDocumentJSONBytesText(v)
	default:
		return strings.TrimSpace(asString(v))
	}
}

func searchDocumentJSONBytesText(raw []byte) string {
	var items []string
	if err := json.Unmarshal(raw, &items); err == nil {
		return joinUniqueSearchParts(items...)
	}
	return strings.TrimSpace(string(raw))
}
