package docprocessing

import llmclients "github.com/chendingplano/shared/go/api/llm"

func topicExtractionContract() llmclients.StructuredOutputContract {
	return llmclients.StructuredOutputContract{
		Name:        "chenweb_topic_extraction",
		AllowRepair: true,
		MaxRetries:  2,
		Schema: []byte(`{
			"type": "object",
			"properties": {
				"topics": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"topic_id": {"type": ["integer", "number", "string"]},
							"topic_type": {"type": "string"},
							"topic_type_en": {"type": "string"},
							"lines": {
								"type": "array",
								"items": {"type": "string"}
							},
							"topic_keywords": {
								"type": "array",
								"items": {"type": "string"}
							},
							"topic_keywords_en": {
								"type": "array",
								"items": {"type": "string"}
							},
							"topic_desc": {"type": "string"},
							"topic_desc_en": {"type": "string"},
							"category_paths": {
								"type": "array"
							},
							"category_paths_en": {
								"type": "array"
							}
						},
						"required": ["topic_desc"],
						"additionalProperties": true
					}
				}
			},
			"required": ["topics"],
			"additionalProperties": false
		}`),
	}
}

func sceneExtractionContract() llmclients.StructuredOutputContract {
	return llmclients.StructuredOutputContract{
		Name:        "chenweb_scene_extraction",
		AllowRepair: true,
		MaxRetries:  2,
		Schema: []byte(`{
			"type": "object",
			"properties": {
				"candidates": { "type": "array" },
				"scene_blocks": { "type": "array" }
			},
			"anyOf": [
				{ "required": ["candidates"] },
				{ "required": ["scene_blocks"] }
			],
			"additionalProperties": false
		}`),
	}
}

func docMetadataExtractionContract() llmclients.StructuredOutputContract {
	return llmclients.StructuredOutputContract{
		Name:        "chenweb_doc_metadata_extraction",
		AllowRepair: true,
		MaxRetries:  2,
		Schema: []byte(`{
			"type": "object",
			"properties": {
				"title": { "type": "string" },
				"doc_no": { "type": "string" },
				"publish_date": { "type": "string" },
				"implementation_date": { "type": "string" },
				"authors": {
					"type": "array",
					"items": { "type": "string" }
				},
				"main_drafting_persons": {
					"type": "array",
					"items": { "type": "string" }
				},
				"drafting_persons": {
					"type": "array",
					"items": { "type": "string" }
				},
				"need_more_pages": { "type": "boolean" }
			},
			"additionalProperties": true
		}`),
	}
}

func metricsExtractionContract() llmclients.StructuredOutputContract {
	return llmclients.StructuredOutputContract{
		Name:        "chenweb_metrics_extraction",
		AllowRepair: true,
		MaxRetries:  2,
		Schema: []byte(`{
			"type": "object",
			"properties": {
				"language": { "type": "string" },
				"candidates": { "type": "array" },
				"metrics": { "type": "array" },
				"uncertain_metrics": { "type": "array" }
			},
			"anyOf": [
				{ "required": ["candidates"] },
				{ "required": ["metrics"] }
			],
			"additionalProperties": true
		}`),
	}
}

func summaryExtractionContract() llmclients.StructuredOutputContract {
	return llmclients.StructuredOutputContract{
		Name:        "chenweb_summary_extraction",
		AllowRepair: true,
		MaxRetries:  2,
		Schema: []byte(`{
			"type": "object",
			"properties": {
				"summary": { "type": "string" },
				"summary_en": { "type": "string" },
				"text": { "type": "string" },
				"keywords": { "type": "array" },
				"keywords_en": { "type": "array" },
				"category_path": { "type": "array" },
				"category_paths": { "type": "array" },
				"category_paths_en": { "type": "array" }
			},
			"additionalProperties": true
		}`),
	}
}

func structureExtractionContract() llmclients.StructuredOutputContract {
	return llmclients.StructuredOutputContract{
		Name:        "chenweb_structure_extraction",
		AllowRepair: true,
		MaxRetries:  2,
		Schema: []byte(`{
			"type": "object",
			"properties": {
				"cover_pages": {
					"type": "array",
					"items": { "type": ["integer", "number", "string"] }
				},
				"labels": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"line_number": { "type": ["integer", "number", "string"] },
							"corrected_line_type": { "type": "string" },
							"confidence": { "type": ["integer", "number", "string"] },
							"reason": { "type": "string" }
						},
						"required": ["line_number", "corrected_line_type", "confidence", "reason"],
						"additionalProperties": true
					}
				}
			},
			"required": ["cover_pages", "labels"],
			"additionalProperties": false
		}`),
	}
}
