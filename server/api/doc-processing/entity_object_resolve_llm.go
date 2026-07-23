package docprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

const (
	defaultEntityObjectResolvePromptRef = "prompt-resolve-entity-object-v1.md"
)

// entityObjectResolveContract is the structured-output schema for the Phase
// 4 classifier, per ADR 2026070101 Phase 4 §Classifier Contract.
func entityObjectResolveContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_entity_object_resolve", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"decision":           schemaString(),
			"confidence":         schemaScalar("number", "integer", "string"),
			"rationale":          schemaString(),
			"selected_object_id": schemaString(),
			"object_type":        schemaString(),
		},
		"required":             []string{"decision", "confidence"},
		"additionalProperties": false,
	})
}

// entityObjectClassifierLLMInput builds the classifier's input payload: the
// entity's document-independent identity signature (the same field set ADR
// 2026061701's entity-dedup adjudicator uses) plus the current
// kb.object_nodes candidates, so the model can distinguish "associate with
// an existing node" from "associate, but nothing exists yet."
func entityObjectClassifierLLMInput(e pendingEntityRow, candidates []ObjectNodeCandidate) map[string]any {
	candidateInputs := make([]map[string]any, 0, len(candidates))
	for _, c := range candidates {
		candidateInputs = append(candidateInputs, map[string]any{
			"object_id":         c.Node.ObjectID,
			"canonical_name":    c.Node.CanonicalName,
			"canonical_name_en": c.Node.CanonicalNameEn,
			"object_type":       c.Node.ObjectType,
			"description":       c.Node.Description,
			"score":             c.Score,
		})
	}
	return map[string]any{
		"entity": map[string]any{
			"entity":      e.Entity,
			"entity_en":   e.EntityEN,
			"entity_type": firstNonEmptyTrimmed(e.EntityTypeEN, e.EntityType),
			"aliases":     e.Aliases,
			"aliases_en":  e.AliasesEN,
			"description": firstNonEmptyTrimmed(e.DescEN, e.Desc),
			"keywords":    e.Keywords,
			"categories":  e.Categories,
		},
		"object_nodes": candidateInputs,
	}
}

// parseEntityObjectClassification unpacks and validates the classifier's raw
// JSON payload into EntityObjectClassification. Confidence parsing is
// tolerant of qualitative labels, mirroring ADR 2026070701 DR7's hardening
// (a non-numeric confidence must never silently collapse to 0, which would
// trip the missing-confidence guard).
func parseEntityObjectClassification(payload map[string]any, modelName string) (EntityObjectClassification, error) {
	if payload == nil {
		return EntityObjectClassification{}, fmt.Errorf("empty entity object classification payload")
	}
	decision := strings.ToLower(strings.TrimSpace(asString(payload["decision"])))
	if decision == "" {
		return EntityObjectClassification{}, fmt.Errorf("entity object classification payload missing decision")
	}
	confidence := parseConfidence(payload["confidence"])
	return EntityObjectClassification{
		Decision:         decision,
		Confidence:       confidence,
		Rationale:        strings.TrimSpace(asString(payload["rationale"])),
		SelectedObjectID: strings.TrimSpace(asString(payload["selected_object_id"])),
		ObjectType:       strings.TrimSpace(asString(payload["object_type"])),
		ModelName:        strings.TrimSpace(modelName),
	}, nil
}

// entityObjectClassifierJSONResolver implements EntityObjectClassifier over
// the shared LLM JSON client, mirroring ambiguousObjectLLMJSONResolver's
// shape (object_ambiguous_llm.go) for the same reasons: consistent call
// construction, consistent structured-output-with-fallback handling.
type entityObjectClassifierJSONResolver struct {
	client    LLMJSONExtractor
	modelName string
}

// NewEntityObjectClassifierFromEnv builds the classifier and its config from
// env, mirroring NewAmbiguousObjectLLMResolverFromEnv. Returns a nil
// classifier (not an error) when ENTITY_OBJECT_RESOLVE_MODEL_NAME is unset,
// so callers can treat "not configured" as a normal, disableable state.
func NewEntityObjectClassifierFromEnv() (EntityObjectClassifier, EntityObjectResolveConfig, error) {
	cfg := EntityObjectResolveConfig{
		MinConfidence: envFloat("ENTITY_OBJECT_RESOLVE_MIN_CONFIDENCE", defaultEntityObjectResolveMinConfidence, 0),
		MaxAttempts:   envInt("ENTITY_OBJECT_RESOLVE_MAX_ATTEMPTS", defaultEntityObjectResolveMaxAttempts, 1),
	}
	modelRef := strings.TrimSpace(os.Getenv("ENTITY_OBJECT_RESOLVE_MODEL_NAME"))
	if modelRef == "" {
		return nil, cfg, nil
	}
	client, modelName, err := BuildReviewerLLMClient(modelRef)
	if err != nil {
		return nil, cfg, err
	}
	return entityObjectClassifierJSONResolver{client: client, modelName: modelName}, cfg, nil
}

func (r entityObjectClassifierJSONResolver) ClassifyEntityForObjectLink(ctx context.Context, e pendingEntityRow, candidates []ObjectNodeCandidate) (EntityObjectClassification, error) {
	if r.client == nil {
		return EntityObjectClassification{}, fmt.Errorf("entity object classifier LLM client is nil")
	}
	input, err := json.Marshal(entityObjectClassifierLLMInput(e, candidates))
	if err != nil {
		return EntityObjectClassification{}, err
	}
	promptText, _, _, promptErr := loadPromptByRef(entityObjectResolvePromptRefFromEnv())
	if promptErr != nil {
		return EntityObjectClassification{}, promptErr
	}
	in := newLLMJSONInput(ctx,
		"resolve_entity_object",
		promptText,
		r.modelName,
		string(input),
		"resolve_entity_object",
		"MID-CWB-ENTITY-OBJECT-RESOLVE",
	)

	var payload map[string]any
	if structured, ok := r.client.(LLMStructuredJSONExtractor); ok {
		result, err := structured.ExtractStructuredJSON(ctx, in, entityObjectResolveContract())
		if err != nil {
			return EntityObjectClassification{}, err
		}
		payload = result.Parsed
	} else {
		payload, err = r.client.ExtractJSON(ctx, in)
		if err != nil {
			return EntityObjectClassification{}, err
		}
	}
	return parseEntityObjectClassification(payload, r.modelName)
}

func entityObjectResolvePromptRefFromEnv() string {
	return firstNonEmptyTrimmed(strings.TrimSpace(os.Getenv("ENTITY_OBJECT_RESOLVE_PROMPT")), defaultEntityObjectResolvePromptRef)
}
