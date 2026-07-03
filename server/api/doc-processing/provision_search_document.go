package docprocessing

import "strings"

func buildProvisionSearchDocument(provision map[string]any, includeEnglish bool) string {
	parts := []string{
		strings.TrimSpace(asString(provision["prov_name"])),
		"",
		strings.TrimSpace(asString(provision["provision_type"])),
		strings.TrimSpace(asString(provision["source_text"])),
		strings.TrimSpace(asString(provision["provision"])),
		"",
		strings.TrimSpace(asString(provision["provision_subject"])),
		"",
		strings.TrimSpace(asString(provision["prov_desc"])),
		"",
		strings.TrimSpace(asString(provision["prov_context"])),
		"",
		searchDocumentArrayText(provision["provision_keywords"]),
		"",
		searchDocumentArrayText(provision["category_paths"]),
		"",
		searchDocumentObjectsText(provision["objects"]),
	}
	if includeEnglish {
		parts[1] = strings.TrimSpace(asString(provision["prov_name_en"]))
		parts[5] = strings.TrimSpace(asString(provision["provision_en"]))
		parts[7] = strings.TrimSpace(asString(provision["provision_subject_en"]))
		parts[9] = strings.TrimSpace(asString(provision["prov_desc_en"]))
		parts[11] = strings.TrimSpace(asString(provision["prov_context_en"]))
		parts[13] = searchDocumentArrayText(provision["provision_keywords_en"])
		parts[15] = searchDocumentArrayText(provision["category_paths_en"])
	}
	return joinUniqueSearchParts(parts...)
}
