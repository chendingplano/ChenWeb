package docprocessing

import (
	"encoding/json"
	"fmt"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

const defaultStructuredContractRetries = 2

func topicExtractionContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_topic_extraction", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"topics": schemaArrayOfObjects(
				map[string]any{
					"topic_id":          schemaScalar("integer", "number", "string"),
					"topic_type":        schemaString(),
					"topic_type_en":     schemaString(),
					"lines":             schemaStringArray(),
					"topic_keywords":    schemaStringArray(),
					"topic_keywords_en": schemaStringArray(),
					"topic_desc":        schemaString(),
					"topic_desc_en":     schemaString(),
					"category_paths":    schemaArray(),
					"category_paths_en": schemaArray(),
				},
				[]string{"topic_desc"},
				true,
			),
		},
		"required":             []string{"topics"},
		"additionalProperties": false,
	})
}

func sceneExtractionContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_scene_extraction", schemaObjectWithAnyOf(
		map[string]any{
			"candidates":   schemaArray(),
			"scene_blocks": schemaArray(),
		},
		[]string{"candidates"},
		[]string{"scene_blocks"},
		false,
	))
}

func docMetadataExtractionContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_doc_metadata_extraction", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":                 schemaString(),
			"doc_no":                schemaString(),
			"publish_date":          schemaString(),
			"implementation_date":   schemaString(),
			"authors":               schemaStringArray(),
			"main_drafting_persons": schemaStringArray(),
			"drafting_persons":      schemaStringArray(),
			"need_more_pages":       map[string]any{"type": "boolean"},
		},
		"additionalProperties": true,
	})
}

func metricsExtractionContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_metrics_extraction", schemaObjectWithAnyOf(
		map[string]any{
			"language":          schemaString(),
			"candidates":        schemaArray(),
			"metrics":           schemaArray(),
			"uncertain_metrics": schemaArray(),
		},
		[]string{"candidates"},
		[]string{"metrics"},
		true,
	))
}

func summaryExtractionContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_summary_extraction", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary":           schemaString(),
			"summary_en":        schemaString(),
			"text":              schemaString(),
			"keywords":          schemaArray(),
			"keywords_en":       schemaArray(),
			"category_path":     schemaArray(),
			"category_paths":    schemaArray(),
			"category_paths_en": schemaArray(),
		},
		"additionalProperties": true,
	})
}

func summaryCategoryTranslationContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_summary_category_translation", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"category_paths_en": schemaArray(),
		},
		"required":             []string{"category_paths_en"},
		"additionalProperties": false,
	})
}

func summaryTextTranslationContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_summary_text_translation", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": schemaString(),
		},
		"required":             []string{"summary"},
		"additionalProperties": false,
	})
}

func summaryKeywordsTranslationContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_summary_keywords_translation", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"keywords": schemaStringArray(),
		},
		"required":             []string{"keywords"},
		"additionalProperties": false,
	})
}

func structureExtractionContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_structure_extraction", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"cover_pages": schemaArrayOf(map[string]any{
				"type": []string{"integer", "number", "string"},
			}),
			"labels": schemaArrayOfObjects(
				map[string]any{
					"line_number":         schemaScalar("integer", "number", "string"),
					"corrected_line_type": schemaString(),
					"confidence":          schemaScalar("integer", "number", "string"),
					"reason":              schemaString(),
				},
				[]string{"line_number", "corrected_line_type", "confidence", "reason"},
				true,
			),
		},
		"required":             []string{"cover_pages", "labels"},
		"additionalProperties": false,
	})
}

func provisionExtractionContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_provision_extraction", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"language":   schemaString(),
			"provisions": schemaArray(),
		},
		"required":             []string{"provisions"},
		"additionalProperties": true,
	})
}

func semanticProjectionExtractionContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_semantic_projection_extraction", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"semantic_projection":    schemaString(),
			"semantic_projection_en": schemaString(),
			"language":               schemaString(),
			"descriptive_name":       schemaString(),
			"descriptive_name_en":    schemaString(),
			"keywords":               schemaArray(),
			"keywords_en":            schemaArray(),
			"category_paths":         schemaArray(),
			"category_paths_en":      schemaArray(),
		},
		"additionalProperties": true,
	})
}

func structuredKnowledgeCandidateContract() llmclients.StructuredOutputContract {
	knowledgeItemSchema := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
	}
	return newDocProcessingContract("chenweb_structured_knowledge_candidate", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"entities":                 knowledgeItemSchema,
			"concepts":                 knowledgeItemSchema,
			"relationships":            knowledgeItemSchema,
			"normative_statements":     knowledgeItemSchema,
			"quantitative_constraints": knowledgeItemSchema,
			"temporal_constraints":     knowledgeItemSchema,
			"conditional_logic":        knowledgeItemSchema,
			"causal_relationships":     knowledgeItemSchema,
			"assumptions":              knowledgeItemSchema,
			"references":               knowledgeItemSchema,
			"procedures":               knowledgeItemSchema,
			"ambiguities":              knowledgeItemSchema,
		},
		"additionalProperties": true,
	})
}

func structuredKnowledgeEnrichContract() llmclients.StructuredOutputContract {
	knowledgeItemSchema := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
	}
	return newDocProcessingContract("chenweb_structured_knowledge_enrich", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"language":                    schemaString(),
			"entities":                    knowledgeItemSchema,
			"entities_en":                 knowledgeItemSchema,
			"concepts":                    knowledgeItemSchema,
			"concepts_en":                 knowledgeItemSchema,
			"relationships":               knowledgeItemSchema,
			"relationships_en":            knowledgeItemSchema,
			"normative_statements":        knowledgeItemSchema,
			"normative_statements_en":     knowledgeItemSchema,
			"quantitative_constraints":    knowledgeItemSchema,
			"quantitative_constraints_en": knowledgeItemSchema,
			"temporal_constraints":        knowledgeItemSchema,
			"temporal_constraints_en":     knowledgeItemSchema,
			"conditional_logic":           knowledgeItemSchema,
			"conditional_logic_en":        knowledgeItemSchema,
			"causal_relationships":        knowledgeItemSchema,
			"causal_relationships_en":     knowledgeItemSchema,
			"assumptions":                 knowledgeItemSchema,
			"assumptions_en":              knowledgeItemSchema,
			"references":                  knowledgeItemSchema,
			"references_en":               knowledgeItemSchema,
			"procedures":                  knowledgeItemSchema,
			"procedures_en":               knowledgeItemSchema,
			"ambiguities":                 knowledgeItemSchema,
			"ambiguities_en":              knowledgeItemSchema,
			"category_paths":              schemaArray(),
			"category_paths_en":           schemaArray(),
		},
		"additionalProperties": true,
	})
}

// entityExtractionContract is the Phase 1 (ADR 2026061302 D2) contract: the
// model extracts entities only. Relations are produced by the separate Phase 2
// pass (relationExtractionContract), so the schema constrains output to
// {language, entities} and saves the relation-output tokens Phase 1 used to spend.
func entityExtractionContract() llmclients.StructuredOutputContract {
	itemArray := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
	}
	return newDocProcessingContract("chenweb_entity_extraction", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"language": schemaString(),
			"entities": itemArray,
		},
		"additionalProperties": true,
	})
}

// relationExtractionContract is the Phase 2 (ADR 2026061302 D5) contract: the
// model receives a window's entity list and returns only relations among them.
func relationExtractionContract() llmclients.StructuredOutputContract {
	itemArray := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
	}
	return newDocProcessingContract("chenweb_relation_extraction", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"language":  schemaString(),
			"relations": itemArray,
		},
		"additionalProperties": true,
	})
}

func productExtractionContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_product_extraction", schemaObjectWithAnyOf(
		map[string]any{
			"products": schemaArray(),
			"mentions": schemaArray(),
		},
		[]string{"products"},
		[]string{"mentions"},
		true,
	))
}

func tocDetectionContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_toc_detection", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"toc_line_numbers": schemaStringArray(),
		},
		"required":             []string{"toc_line_numbers"},
		"additionalProperties": false,
	})
}

func newDocProcessingContract(name string, schema map[string]any) llmclients.StructuredOutputContract {
	return llmclients.StructuredOutputContract{
		Name:        name,
		AllowRepair: true,
		MaxRetries:  defaultStructuredContractRetries,
		Schema:      mustMarshalSchema(schema),
	}
}

func mustMarshalSchema(schema map[string]any) []byte {
	bs, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("marshal structured output schema: %v", err))
	}
	return bs
}

func schemaString() map[string]any {
	return map[string]any{"type": "string"}
}

func schemaArray() map[string]any {
	return map[string]any{"type": "array"}
}

func schemaStringArray() map[string]any {
	return schemaArrayOf(schemaString())
}

func schemaArrayOf(itemSchema map[string]any) map[string]any {
	return map[string]any{
		"type":  "array",
		"items": itemSchema,
	}
}

func schemaArrayOfObjects(properties map[string]any, required []string, additionalProperties bool) map[string]any {
	return schemaArrayOf(map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": additionalProperties,
	})
}

func schemaScalar(types ...string) map[string]any {
	if len(types) == 1 {
		return map[string]any{"type": types[0]}
	}
	return map[string]any{"type": types}
}

func schemaObjectWithAnyOf(properties map[string]any, requiredA []string, requiredB []string, additionalProperties bool) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": properties,
		"anyOf": []map[string]any{
			{"required": requiredA},
			{"required": requiredB},
		},
		"additionalProperties": additionalProperties,
	}
}
