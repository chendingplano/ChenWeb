package docprocessing

import (
	"maps"
	"strings"
)

// expandCompactProvisionPayload restores only explicitly marked wire responses.
// Copy maps before filling defaults so captured provider data stays untouched.
func expandCompactProvisionPayload(payload map[string]any, items []any) map[string]any {
	if payload["output_format"] != "compact-v1" {
		payload["provisions"] = items
		return payload
	}
	out := maps.Clone(payload)
	expanded := make([]any, len(items))
	language := strings.ToLower(strings.TrimSpace(asString(payload["language"])))
	isChinese := language == "zh" || strings.HasPrefix(language, "zh-") || language == "chinese"
	for i, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			expanded[i] = item
			continue
		}
		provision := maps.Clone(raw)
		for _, key := range []string{"name", "name_en", "type", "provision", "provision_en", "provision_desc", "provision_desc_en", "context", "context_en", "subject", "subject_en", "location_type"} {
			setMissingProvisionField(provision, key, "")
		}
		for _, key := range []string{"keywords", "keywords_en"} {
			setMissingProvisionField(provision, key, []any{})
		}
		objects := objectItemsFromValue(raw["objects"])
		expandedObjects := make([]any, 0, len(objects))
		for _, rawObject := range objects {
			object := maps.Clone(rawObject)
			nameZh := ""
			if isChinese {
				nameZh = asString(rawObject["object_name"])
			}
			setMissingProvisionField(object, "object_name_zh", nameZh)
			setMissingProvisionField(object, "source_line_spans", provision["source_line_spans"])
			setMissingProvisionField(object, "confidence", provision["confidence"])
			for _, key := range []string{"object_name", "object_name_en", "object_type", "object_role", "description", "evidence_quote"} {
				setMissingProvisionField(object, key, "")
			}
			for _, key := range []string{"aliases", "acronyms"} {
				setMissingProvisionField(object, key, []any{})
			}
			expandedObjects = append(expandedObjects, object)
		}
		if _, present := raw["objects"]; !present || objects != nil {
			provision["objects"] = expandedObjects
		}
		expanded[i] = provision
	}
	out["provisions"] = expanded
	return out
}

func setMissingProvisionField(record map[string]any, key string, value any) {
	if _, present := record[key]; !present {
		record[key] = value
	}
}
