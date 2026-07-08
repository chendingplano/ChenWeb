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
	ObjectReconcileMethodLLMAmbiguous = "llm_ambiguous_resolution"
	defaultResolveAmbiguousMinConf    = 0.85
)

// AmbiguousArtifactObjectLLMUpdate is the allowlisted set of
// kb.artifact_objects fields DR7 permits the LLM to complete/correct.
type AmbiguousArtifactObjectLLMUpdate struct {
	ObjectNameEn string
	ObjectNameZh string
	Acronyms     []string
	ObjectType   string
	Description  string
}

// AmbiguousObjectNodeLLMUpdate is the allowlisted set of kb.object_nodes fields
// DR7 permits the LLM to complete/correct.
type AmbiguousObjectNodeLLMUpdate struct {
	ObjectID        string
	CanonicalNameEn string
	CanonicalNameZh string
	ObjectType      string
	Description     string
}

// AmbiguousObjectNodeLLMMerge describes one survivor and one or more loser
// object ids that the LLM judged to represent the same object.
type AmbiguousObjectNodeLLMMerge struct {
	SurvivorObjectID string
	LoserObjectIDs   []string
	Confidence       float64
}

// AmbiguousObjectLLMDecision is the validated shape produced by the DR7 LLM
// adjudicator before it is applied to artifact/object-node state.
type AmbiguousObjectLLMDecision struct {
	ModelName            string
	ResolutionObjectID   string
	ResolutionConfidence float64
	ArtifactUpdates      AmbiguousArtifactObjectLLMUpdate
	NodeUpdates          []AmbiguousObjectNodeLLMUpdate
	Merges               []AmbiguousObjectNodeLLMMerge
	Rationale            string
}

type AmbiguousObjectLLMAudit struct {
	ModelName string
	Rationale string
}

type AmbiguousObjectLLMApplyStore interface {
	ApplyAmbiguousObjectLLMNodeChanges(ctx context.Context, obj ArtifactObject, updates []AmbiguousObjectNodeLLMUpdate, merges []AmbiguousObjectNodeLLMMerge, audit AmbiguousObjectLLMAudit) error
}

type ambiguousObjectLLMJSONResolver struct {
	client    LLMJSONExtractor
	modelName string
}

func NewAmbiguousObjectLLMResolverFromEnv() (AmbiguousObjectLLMResolver, float64, error) {
	minConfidence := envFloat("RESOLVE_AMBIGUOUS_MIN_CONFIDENCE", defaultResolveAmbiguousMinConf, 0)
	modelRef := strings.TrimSpace(os.Getenv("RESOLVE_AMBIGUOUS_OBJECT_MODEL_NAME"))
	if modelRef == "" {
		return nil, minConfidence, nil
	}
	client, modelName, err := BuildReviewerLLMClient(modelRef)
	if err != nil {
		return nil, minConfidence, err
	}
	return ambiguousObjectLLMJSONResolver{client: client, modelName: modelName}, minConfidence, nil
}

func (r ambiguousObjectLLMJSONResolver) ResolveAmbiguousObject(ctx context.Context, obj ArtifactObject, candidates []ObjectNodeCandidate) (AmbiguousObjectLLMDecision, error) {
	if r.client == nil {
		return AmbiguousObjectLLMDecision{}, fmt.Errorf("ambiguous object LLM client is nil")
	}
	input, err := json.Marshal(map[string]any{
		"artifact_object": ambiguousArtifactObjectLLMInput(obj),
		"object_nodes":    ambiguousObjectNodeCandidateLLMInput(candidates),
	})
	if err != nil {
		return AmbiguousObjectLLMDecision{}, err
	}
	in := newLLMJSONInput(ctx,
		"resolve_ambiguous_object",
		ambiguousObjectLLMPrompt(),
		r.modelName,
		string(input),
		"resolve_ambiguous_object",
		"MID-CWB-OBJECT-AMBIGUOUS-LLM",
	)
	var payload map[string]any
	if structured, ok := r.client.(LLMStructuredJSONExtractor); ok {
		result, err := structured.ExtractStructuredJSON(ctx, in, ambiguousObjectResolutionContract())
		if err != nil {
			return AmbiguousObjectLLMDecision{}, err
		}
		payload = result.Parsed
	} else {
		payload, err = r.client.ExtractJSON(ctx, in)
		if err != nil {
			return AmbiguousObjectLLMDecision{}, err
		}
	}
	return parseAmbiguousObjectLLMDecision(payload, r.modelName)
}

// ApplyAmbiguousObjectLLMDecision validates and applies one DR7 LLM decision to
// an in-memory artifact object, and delegates object-node edits/merges to store
// when supplied. Low-confidence decisions intentionally leave the object in the
// existing ambiguous backlog for DR5/DR6.
func ApplyAmbiguousObjectLLMDecision(ctx context.Context, obj ArtifactObject, candidates []ObjectNodeCandidate, decision AmbiguousObjectLLMDecision, store AmbiguousObjectLLMApplyStore, minConfidence float64) (ArtifactObject, bool, error) {
	if minConfidence <= 0 {
		minConfidence = defaultResolveAmbiguousMinConf
	}
	candidateIDs := candidateObjectIDSet(candidates)
	if len(candidateIDs) == 0 {
		return obj, false, fmt.Errorf("LLM ambiguous resolution has no candidates")
	}

	selectedID := strings.TrimSpace(decision.ResolutionObjectID)
	if selectedID == "" {
		return obj, false, fmt.Errorf("LLM ambiguous resolution missing selected object_id")
	}
	if _, ok := candidateIDs[selectedID]; !ok {
		return obj, false, fmt.Errorf("LLM ambiguous resolution selected unknown object_id %q", selectedID)
	}

	merges, loserToSurvivor, err := validateAmbiguousObjectLLMMerges(decision.Merges, candidateIDs)
	if err != nil {
		return obj, false, err
	}
	updates, err := validateAmbiguousObjectLLMNodeUpdates(decision.NodeUpdates, candidateIDs)
	if err != nil {
		return obj, false, err
	}

	if decision.ResolutionConfidence < minConfidence {
		obj.ObjectID = ""
		obj.ReconcileStatus = ObjectReconcileAmbiguous
		obj.ReconcileConfidence = decision.ResolutionConfidence
		return obj, false, nil
	}

	resolvedID := selectedID
	if survivor := loserToSurvivor[selectedID]; survivor != "" {
		resolvedID = survivor
	}

	if store != nil && (len(updates) > 0 || len(merges) > 0) {
		if err := store.ApplyAmbiguousObjectLLMNodeChanges(ctx, obj, updates, merges, AmbiguousObjectLLMAudit{
			ModelName: strings.TrimSpace(decision.ModelName),
			Rationale: strings.TrimSpace(decision.Rationale),
		}); err != nil {
			return obj, false, err
		}
	}

	obj = applyAmbiguousArtifactObjectLLMUpdate(obj, decision.ArtifactUpdates)
	obj.ObjectID = resolvedID
	obj.ReconcileStatus = ObjectReconcileAmbiguousResolved
	obj.ReconcileConfidence = decision.ResolutionConfidence
	if obj.ExtInfo == nil {
		obj.ExtInfo = map[string]any{}
	}
	obj.ExtInfo["reconcile_method"] = ObjectReconcileMethodLLMAmbiguous
	if modelName := strings.TrimSpace(decision.ModelName); modelName != "" {
		obj.ExtInfo["reconcile_model"] = modelName
	}
	return obj, true, nil
}

func candidateObjectIDSet(candidates []ObjectNodeCandidate) map[string]struct{} {
	out := make(map[string]struct{}, len(candidates))
	for _, c := range candidates {
		if id := strings.TrimSpace(c.Node.ObjectID); id != "" {
			out[id] = struct{}{}
		}
	}
	return out
}

func validateAmbiguousObjectLLMNodeUpdates(updates []AmbiguousObjectNodeLLMUpdate, candidateIDs map[string]struct{}) ([]AmbiguousObjectNodeLLMUpdate, error) {
	out := make([]AmbiguousObjectNodeLLMUpdate, 0, len(updates))
	for _, u := range updates {
		u.ObjectID = strings.TrimSpace(u.ObjectID)
		if u.ObjectID == "" {
			return nil, fmt.Errorf("LLM object-node update missing object_id")
		}
		if _, ok := candidateIDs[u.ObjectID]; !ok {
			return nil, fmt.Errorf("LLM object-node update references unknown object_id %q", u.ObjectID)
		}
		u.CanonicalNameEn = strings.TrimSpace(u.CanonicalNameEn)
		u.CanonicalNameZh = strings.TrimSpace(u.CanonicalNameZh)
		u.ObjectType = normalizeObjectToken(u.ObjectType)
		u.Description = strings.TrimSpace(u.Description)
		out = append(out, u)
	}
	return out, nil
}

func validateAmbiguousObjectLLMMerges(merges []AmbiguousObjectNodeLLMMerge, candidateIDs map[string]struct{}) ([]AmbiguousObjectNodeLLMMerge, map[string]string, error) {
	out := make([]AmbiguousObjectNodeLLMMerge, 0, len(merges))
	loserToSurvivor := map[string]string{}
	for _, m := range merges {
		m.SurvivorObjectID = strings.TrimSpace(m.SurvivorObjectID)
		if m.SurvivorObjectID == "" {
			return nil, nil, fmt.Errorf("LLM object-node merge missing survivor_object_id")
		}
		if _, ok := candidateIDs[m.SurvivorObjectID]; !ok {
			return nil, nil, fmt.Errorf("LLM object-node merge references unknown survivor_object_id %q", m.SurvivorObjectID)
		}
		losers := make([]string, 0, len(m.LoserObjectIDs))
		for _, rawLoser := range m.LoserObjectIDs {
			loser := strings.TrimSpace(rawLoser)
			if loser == "" {
				return nil, nil, fmt.Errorf("LLM object-node merge has empty loser_object_id")
			}
			if loser == m.SurvivorObjectID {
				return nil, nil, fmt.Errorf("LLM object-node merge loser and survivor are both %q", loser)
			}
			if _, ok := candidateIDs[loser]; !ok {
				return nil, nil, fmt.Errorf("LLM object-node merge references unknown loser_object_id %q", loser)
			}
			if existing := loserToSurvivor[loser]; existing != "" && existing != m.SurvivorObjectID {
				return nil, nil, fmt.Errorf("LLM object-node merge loser %q maps to multiple survivors", loser)
			}
			loserToSurvivor[loser] = m.SurvivorObjectID
			losers = append(losers, loser)
		}
		if len(losers) == 0 {
			return nil, nil, fmt.Errorf("LLM object-node merge for survivor %q has no losers", m.SurvivorObjectID)
		}
		m.LoserObjectIDs = uniqueStrings(losers)
		out = append(out, m)
	}
	return out, loserToSurvivor, nil
}

func applyAmbiguousArtifactObjectLLMUpdate(obj ArtifactObject, update AmbiguousArtifactObjectLLMUpdate) ArtifactObject {
	if v := strings.TrimSpace(update.ObjectNameEn); v != "" {
		obj.ObjectNameEn = v
	}
	if v := strings.TrimSpace(update.ObjectNameZh); v != "" {
		obj.ObjectNameZh = v
	}
	if len(update.Acronyms) > 0 {
		obj.Acronyms = uniqueStrings(append(obj.Acronyms, update.Acronyms...))
	}
	if v := normalizeObjectToken(update.ObjectType); v != "" {
		obj.ObjectType = v
	}
	if v := strings.TrimSpace(update.Description); v != "" {
		obj.Description = v
	}
	obj.NormalizedNames = buildObjectNormalizedNames(obj, obj.NormalizedNames)
	return obj
}

func parseAmbiguousObjectLLMDecision(payload map[string]any, modelName string) (AmbiguousObjectLLMDecision, error) {
	if payload == nil {
		return AmbiguousObjectLLMDecision{}, fmt.Errorf("empty LLM ambiguous object payload")
	}
	artifactUpdates := AmbiguousArtifactObjectLLMUpdate{}
	if raw, ok := payload["artifact_object"].(map[string]any); ok {
		artifactUpdates = AmbiguousArtifactObjectLLMUpdate{
			ObjectNameEn: strings.TrimSpace(asString(raw["object_name_en"])),
			ObjectNameZh: strings.TrimSpace(asString(raw["object_name_zh"])),
			Acronyms:     uniqueStrings(toStringSlice(raw["acronyms"])),
			ObjectType:   normalizeObjectToken(asString(raw["object_type"])),
			Description:  strings.TrimSpace(asString(raw["description"])),
		}
	}
	nodeUpdates, err := parseAmbiguousObjectLLMNodeUpdates(payload["object_nodes"])
	if err != nil {
		return AmbiguousObjectLLMDecision{}, err
	}
	merges, err := parseAmbiguousObjectLLMMerges(payload["same_object_groups"])
	if err != nil {
		return AmbiguousObjectLLMDecision{}, err
	}
	resolution, ok := payload["resolution"].(map[string]any)
	if !ok {
		return AmbiguousObjectLLMDecision{}, fmt.Errorf("LLM ambiguous object payload missing resolution")
	}
	decision := AmbiguousObjectLLMDecision{
		ModelName:            strings.TrimSpace(modelName),
		ResolutionObjectID:   strings.TrimSpace(asString(resolution["object_id"])),
		ResolutionConfidence: toFloat(resolution["confidence"]),
		ArtifactUpdates:      artifactUpdates,
		NodeUpdates:          nodeUpdates,
		Merges:               merges,
		Rationale:            strings.TrimSpace(asString(payload["rationale"])),
	}
	if decision.ResolutionObjectID == "" {
		return AmbiguousObjectLLMDecision{}, fmt.Errorf("LLM ambiguous object payload missing resolution.object_id")
	}
	if decision.ResolutionConfidence == 0 {
		return AmbiguousObjectLLMDecision{}, fmt.Errorf("LLM ambiguous object payload missing resolution.confidence")
	}
	return decision, nil
}

func parseAmbiguousObjectLLMNodeUpdates(raw any) ([]AmbiguousObjectNodeLLMUpdate, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, nil
	}
	out := make([]AmbiguousObjectNodeLLMUpdate, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("LLM ambiguous object node update is not an object")
		}
		objectID := strings.TrimSpace(asString(m["object_id"]))
		if objectID == "" {
			return nil, fmt.Errorf("LLM ambiguous object node update missing object_id")
		}
		out = append(out, AmbiguousObjectNodeLLMUpdate{
			ObjectID:        objectID,
			CanonicalNameEn: strings.TrimSpace(asString(m["canonical_name_en"])),
			CanonicalNameZh: strings.TrimSpace(asString(m["canonical_name_zh"])),
			ObjectType:      normalizeObjectToken(asString(m["object_type"])),
			Description:     strings.TrimSpace(asString(m["description"])),
		})
	}
	return out, nil
}

func parseAmbiguousObjectLLMMerges(raw any) ([]AmbiguousObjectNodeLLMMerge, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, nil
	}
	out := make([]AmbiguousObjectNodeLLMMerge, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("LLM ambiguous object same-object group is not an object")
		}
		survivor := strings.TrimSpace(asString(m["survivor_object_id"]))
		losers := uniqueStrings(toStringSlice(m["loser_object_ids"]))
		out = append(out, AmbiguousObjectNodeLLMMerge{
			SurvivorObjectID: survivor,
			LoserObjectIDs:   losers,
			Confidence:       toFloat(m["confidence"]),
		})
	}
	return out, nil
}

func ambiguousArtifactObjectLLMInput(obj ArtifactObject) map[string]any {
	return map[string]any{
		"artifact_type":    obj.ArtifactType,
		"artifact_id":      obj.ArtifactID,
		"object_name":      obj.ObjectName,
		"object_name_en":   obj.ObjectNameEn,
		"object_name_zh":   obj.ObjectNameZh,
		"object_type":      obj.ObjectType,
		"object_role":      obj.ObjectRole,
		"aliases":          obj.Aliases,
		"acronyms":         obj.Acronyms,
		"normalized_names": obj.NormalizedNames,
		"description":      obj.Description,
		"evidence_quote":   obj.EvidenceQuote,
	}
}

func ambiguousObjectNodeCandidateLLMInput(candidates []ObjectNodeCandidate) []map[string]any {
	out := make([]map[string]any, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, map[string]any{
			"object_id":         c.Node.ObjectID,
			"canonical_name":    c.Node.CanonicalName,
			"canonical_name_en": c.Node.CanonicalNameEn,
			"canonical_name_zh": c.Node.CanonicalNameZh,
			"object_type":       c.Node.ObjectType,
			"aliases":           c.Node.Aliases,
			"acronyms":          c.Node.Acronyms,
			"normalized_names":  c.Node.NormalizedNames,
			"description":       c.Node.Description,
			"score":             c.Score,
			"method":            c.Method,
		})
	}
	return out
}

func ambiguousObjectLLMPrompt() string {
	return `Resolve one ambiguous artifact object against candidate kb.object_nodes.
Only complete/correct the allowlisted fields. Do not invent object ids.
If candidate nodes represent the same object, return same_object_groups with
one survivor_object_id and loser_object_ids. Return a selected resolution object
id with confidence. Use high confidence only when the semantic identity is clear.`
}

func ambiguousObjectResolutionContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_ambiguous_object_resolution", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"artifact_object": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"object_name_en": schemaString(),
					"object_name_zh": schemaString(),
					"acronyms":       schemaStringArray(),
					"object_type":    schemaString(),
					"description":    schemaString(),
				},
				"additionalProperties": false,
			},
			"object_nodes": schemaArrayOfObjects(map[string]any{
				"object_id":         schemaString(),
				"canonical_name_en": schemaString(),
				"canonical_name_zh": schemaString(),
				"object_type":       schemaString(),
				"description":       schemaString(),
			}, []string{"object_id"}, false),
			"same_object_groups": schemaArrayOfObjects(map[string]any{
				"survivor_object_id": schemaString(),
				"loser_object_ids":   schemaStringArray(),
				"confidence":         schemaScalar("number", "integer", "string"),
			}, []string{"survivor_object_id", "loser_object_ids", "confidence"}, false),
			"resolution": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"object_id":  schemaString(),
					"confidence": schemaScalar("number", "integer", "string"),
				},
				"required":             []string{"object_id", "confidence"},
				"additionalProperties": false,
			},
			"rationale": schemaString(),
		},
		"required":             []string{"resolution"},
		"additionalProperties": false,
	})
}
