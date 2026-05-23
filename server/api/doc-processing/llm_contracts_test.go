package docprocessing

import (
	"encoding/json"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestDocProcessingContracts_AreValid(t *testing.T) {
	contracts := []llmclients.StructuredOutputContract{
		topicExtractionContract(),
		sceneExtractionContract(),
		docMetadataExtractionContract(),
		metricsExtractionContract(),
		summaryExtractionContract(),
		structureExtractionContract(),
		provisionExtractionContract(),
		productExtractionContract(),
	}

	for _, contract := range contracts {
		contract := contract
		t.Run(contract.Name, func(t *testing.T) {
			if err := contract.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			var schema map[string]any
			if err := json.Unmarshal(contract.Schema, &schema); err != nil {
				t.Fatalf("schema is not valid JSON: %v", err)
			}

			if got := schema["type"]; got != "object" {
				t.Fatalf("schema type = %v, want object", got)
			}
		})
	}
}

