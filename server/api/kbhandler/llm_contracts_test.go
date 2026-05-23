package kbhandler

import (
	"encoding/json"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestHandlerContracts_AreValid(t *testing.T) {
	contracts := []llmclients.StructuredOutputContract{
		metricHandlerExtractionContract(),
		provisionHandlerExtractionContract(),
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

