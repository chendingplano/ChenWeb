package docprocessing

var optionalProductionProcessorOrder = []string{
	"generate_summaries",
	"generate_topics",
	"extract_doc_metadata",
	"extract_semantic_projections",
	"extract_structured_knowledge",
	"extract_entity",
	"extract_relation",
	"extract_inventory_items",
	"extract_metrics",
	"extract_provisions",
	"generate_scene_blocks",
}

func OptionalProductionProcessorNames() []string {
	return append([]string(nil), optionalProductionProcessorOrder...)
}

func CanonicalOptionalProductionProcessor(raw string) (string, bool) {
	name := normalizeRuntimeName(raw)
	for _, candidate := range optionalProductionProcessorOrder {
		if candidate == name {
			return candidate, true
		}
	}
	return "", false
}
