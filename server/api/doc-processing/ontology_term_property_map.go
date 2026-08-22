package docprocessing

import "strings"

// ontologyTermPropertyMapping is one parsed entry from
// [ontology_term_property_map].property_map in config.toml/config.local.toml:
// "<artifact_type>:<table_field_name>:<property_name>". It lets an operator
// expose additional already-extracted artifact fields onto the governed
// kb.ontology_terms.properties JSONB bag for an auto-promoted term, without a
// code change per field (bug 2026082101 finding 2 follow-up).
type ontologyTermPropertyMapping struct {
	Field    string // dotted path into the artifact's field map, e.g. "ext_info.object_name"
	Property string // key written into kb.ontology_terms.properties
}

// parseOntologyTermPropertyMap groups the configured "artifact_type:field:property"
// strings by artifact_type. Malformed entries (not exactly 3 ':'-separated
// parts, or an empty artifact_type/field/property) are skipped.
func parseOntologyTermPropertyMap(entries []string) map[string][]ontologyTermPropertyMapping {
	byArtifact := map[string][]ontologyTermPropertyMapping{}
	for _, entry := range entries {
		parts := strings.SplitN(entry, ":", 3)
		if len(parts) != 3 {
			continue
		}
		artifactType := strings.TrimSpace(parts[0])
		field := strings.TrimSpace(parts[1])
		property := strings.TrimSpace(parts[2])
		if artifactType == "" || field == "" || property == "" {
			continue
		}
		byArtifact[artifactType] = append(byArtifact[artifactType], ontologyTermPropertyMapping{Field: field, Property: property})
	}
	return byArtifact
}

// buildOntologyTermProperties applies mappings against an artifact's field
// map (dotted paths resolve into nested maps, e.g. "ext_info.object_name"),
// returning the properties to add to kb.ontology_terms.properties. Absent or
// empty-valued fields are omitted rather than written as null/"".
func buildOntologyTermProperties(mappings []ontologyTermPropertyMapping, m map[string]any) map[string]any {
	if len(mappings) == 0 || len(m) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, mapping := range mappings {
		val := lookupFieldPath(m, mapping.Field)
		if isEmptyFieldValue(val) {
			continue
		}
		out[mapping.Property] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// lookupFieldPath resolves a dot-separated path (e.g. "ext_info.object_name")
// against nested map[string]any values. A missing key or a non-map
// intermediate value yields nil.
func lookupFieldPath(m map[string]any, path string) any {
	var cur any = m
	for _, part := range strings.Split(path, ".") {
		asMap, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = asMap[part]
	}
	return cur
}

func isEmptyFieldValue(v any) bool {
	switch val := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(val) == ""
	case []string:
		return len(val) == 0
	case []any:
		return len(val) == 0
	default:
		return false
	}
}
