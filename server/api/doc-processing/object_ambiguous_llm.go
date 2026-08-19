package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

const (
	ObjectReconcileMethodLLMAmbiguous = "llm_ambiguous_resolution"
	defaultResolveAmbiguousMinConf    = 0.85
	defaultResolveAmbiguousPromptRef  = "prompt-resolve-ambiguous-object-v1.md"
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
	prompt, err := ambiguousObjectLLMPrompt()
	if err != nil {
		return AmbiguousObjectLLMDecision{}, err
	}
	in := newLLMJSONInput(ctx,
		"resolve_ambiguous_object",
		prompt,
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
	decision := AmbiguousObjectLLMDecision{
		ModelName:       strings.TrimSpace(modelName),
		ArtifactUpdates: artifactUpdates,
		NodeUpdates:     nodeUpdates,
		Merges:          merges,
		Rationale:       strings.TrimSpace(asString(payload["rationale"])),
	}
	if resolution, ok := payload["resolution"].(map[string]any); ok {
		decision.ResolutionObjectID = strings.TrimSpace(asString(resolution["object_id"]))
		decision.ResolutionConfidence = parseConfidence(resolution["confidence"])
	} else {
		decision.ResolutionObjectID = strings.TrimSpace(asString(payload["selected_resolution_object_id"]))
		decision.ResolutionConfidence = parseConfidence(payload["confidence"])
	}
	if decision.ResolutionObjectID == "" {
		return AmbiguousObjectLLMDecision{}, fmt.Errorf("LLM ambiguous object payload missing resolution object_id")
	}
	if decision.ResolutionConfidence == 0 {
		return AmbiguousObjectLLMDecision{}, fmt.Errorf("LLM ambiguous object payload missing resolution confidence")
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

// parseConfidence coerces an LLM confidence value to a 0..1 float. It accepts
// numbers and numeric strings ("0.90"), and maps the qualitative labels some
// models emit ("high"/"medium"/"low") to representative scores so a non-numeric
// confidence never silently collapses to zero (which would fail the "missing
// confidence" guard). The prompt asks for numeric confidence; this is defense
// against models that ignore that instruction.
func parseConfidence(v any) float64 {
	if s, ok := v.(string); ok {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "very high", "certain":
			return 0.99
		case "high":
			return 0.9
		case "medium", "moderate":
			return 0.6
		case "low":
			return 0.3
		case "very low":
			return 0.1
		}
	}
	return toFloat(v)
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
			Confidence:       parseConfidence(m["confidence"]),
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

// ambiguousObjectLLMPrompt loads the DR7 adjudicator prompt from the prompts
// directory. The ref is overridable via RESOLVE_AMBIGUOUS_OBJECT_PROMPT so the
// prompt is never hard-coded in the binary.
func ambiguousObjectLLMPrompt() (string, error) {
	promptRef := strings.TrimSpace(os.Getenv("RESOLVE_AMBIGUOUS_OBJECT_PROMPT"))
	if promptRef == "" {
		promptRef = defaultResolveAmbiguousPromptRef
	}
	text, _, _, err := loadPromptByRef(promptRef)
	if err != nil {
		return "", err
	}
	return text, nil
}

// structuredOutputRawResponse returns the raw LLM response captured by a
// structured-output failure, or "" when the error carries no raw payload.
func structuredOutputRawResponse(err error) string {
	var soErr *llmclients.StructuredOutputError
	if errors.As(err, &soErr) {
		return soErr.Raw
	}
	return ""
}

// Reconcile outcome statuses recorded in kb.doc_proc_logs (extra_info.outcome).
const (
	reconcileOutcomeResolved    = "resolved"     // LLM adjudicated and the decision was applied
	reconcileOutcomeUnresolved  = "unresolved"   // LLM answered but confidence < threshold; left ambiguous
	reconcileOutcomeLLMFailed   = "llm_failed"   // ResolveAmbiguousObject returned an error
	reconcileOutcomeApplyFailed = "apply_failed" // decision could not be applied
)

// objectReconcileLogSink persists per-object object-reconciliation outcomes to
// kb.doc_proc_logs. A zero value (nil ProcLogger.DB) disables persistence so
// callers/tests without a database run unchanged.
type objectReconcileLogSink struct {
	ProcLogger  DocProcLogger
	DocProcName string
	CallReason  string
}

func (s objectReconcileLogSink) enabled() bool { return s.ProcLogger.DB != nil }

// objectReconcileOutcome captures one ambiguous artifact object's reconciliation
// outcome so success and failure alike can be logged uniformly.
type objectReconcileOutcome struct {
	Status           string
	Object           ArtifactObject
	Candidates       []ObjectNodeCandidate
	CandidateDisplay []string
	Decision         AmbiguousObjectLLMDecision
	ResolvedID       string
	Err              error
	MSUsed           int64
}

// logReconcileOutcome writes one reconcile_object row. It is best-effort: a
// logging failure is reported via logger but never aborts reconciliation.
func (s objectReconcileLogSink) logReconcileOutcome(ctx context.Context, logger ApiTypes.JimoLogger, o objectReconcileOutcome) {
	if !s.enabled() {
		return
	}
	candidateIDs := make([]string, 0, len(o.Candidates))
	for _, c := range o.Candidates {
		candidateIDs = append(candidateIDs, c.Node.ObjectID)
	}
	extra := map[string]any{
		"outcome":               o.Status,
		"artifact_id":           o.Object.ArtifactID,
		"artifact_type":         o.Object.ArtifactType,
		"object_name":           o.Object.ObjectName,
		"candidate_object_ids":  candidateIDs,
		"candidates":            o.CandidateDisplay,
		"resolved_object_id":    o.ResolvedID,
		"resolution_confidence": o.Decision.ResolutionConfidence,
	}
	extraStr := "{}"
	if bs, err := json.Marshal(extra); err == nil {
		extraStr = string(bs)
	}
	var errStr *string
	if o.Err != nil {
		errStr = nullableStringPtr(o.Err.Error())
	}
	var modelNames []string
	if m := strings.TrimSpace(o.Decision.ModelName); m != "" {
		modelNames = []string{m}
	}
	recordID := o.Object.InputRecordID
	activity := "resolve_ambiguous_object"
	rec := DocProcLogRecord{
		CallReason:    s.CallReason,
		DocProcName:   s.DocProcName,
		ModelNames:    modelNames,
		RecordID:      &recordID,
		ActivityName:  &activity,
		Errors:        errStr,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(o.MSUsed),
	}
	if err := s.ProcLogger.LogReconcileObject(ctx, rec, "MID-CWB-RECONCILE-OBJECT"); err != nil && logger != nil {
		logger.Warn("failed to write reconcile_object log", "artifact_id", o.Object.ArtifactID, "err", err)
	}
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
