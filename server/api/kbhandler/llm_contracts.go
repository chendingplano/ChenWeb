package kbhandler

import (
	"context"
	"encoding/json"
	"fmt"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

const defaultHandlerStructuredRetries = 2

type structuredJSONExtractor interface {
	ExtractStructuredJSON(ctx context.Context, in llmclients.JSONExtractionInput, contract llmclients.StructuredOutputContract) (*llmclients.StructuredOutputResult, error)
}

func metricHandlerExtractionContract() llmclients.StructuredOutputContract {
	return newHandlerContract("chenweb_metric_handler_extraction", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"metrics": map[string]any{"type": "array"},
		},
		"required":             []string{"metrics"},
		"additionalProperties": true,
	})
}

func provisionHandlerExtractionContract() llmclients.StructuredOutputContract {
	return newHandlerContract("chenweb_provision_handler_extraction", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"language":   map[string]any{"type": "string"},
			"provisions": map[string]any{"type": "array"},
		},
		"required":             []string{"provisions"},
		"additionalProperties": true,
	})
}

func newHandlerContract(name string, schema map[string]any) llmclients.StructuredOutputContract {
	return llmclients.StructuredOutputContract{
		Name:        name,
		AllowRepair: true,
		MaxRetries:  defaultHandlerStructuredRetries,
		Schema:      mustMarshalHandlerSchema(schema),
	}
}

func mustMarshalHandlerSchema(schema map[string]any) []byte {
	bs, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("marshal handler structured output schema: %v", err))
	}
	return bs
}
