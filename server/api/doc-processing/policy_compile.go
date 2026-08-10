package docprocessing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

type PolicyClearanceReference struct {
	SubjectKind, SubjectChecksum string
	SubjectID                    int64
}

type PolicyDefinition struct {
	ID                  int64
	Version             int
	Pipelines           []ProductionPipelineSpec
	KnownProcessors     []string
	Bindings            []PipelineBinding
	Gates               []PipelineGate
	ClearanceReferences []PolicyClearanceReference
}

type CompiledPolicy struct {
	PolicyID         int64    `json:"policy_id"`
	Version          int      `json:"version"`
	Checksum         string   `json:"checksum"`
	BindingChecksums []string `json:"binding_checksums,omitempty"`
	GateChecksums    []string `json:"gate_checksums,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
}

func CompilePolicy(definition PolicyDefinition) (CompiledPolicy, error) {
	if definition.ID <= 0 || definition.Version <= 0 {
		return CompiledPolicy{}, errors.New("policy id and version must be positive")
	}
	pipelines := make(map[string]ProductionPipelineSpec, len(definition.Pipelines))
	for _, pipeline := range definition.Pipelines {
		if strings.TrimSpace(pipeline.Name) == "" {
			return CompiledPolicy{}, errors.New("pipeline name is required")
		}
		pipelines[pipeline.Name] = pipeline
	}
	processors := stringSet(definition.KnownProcessors)
	if len(processors) == 0 {
		for _, spec := range productionProcessorSpecs {
			processors[spec.Name] = true
		}
	}
	bindings := append([]PipelineBinding(nil), definition.Bindings...)
	gates := append([]PipelineGate(nil), definition.Gates...)
	compiled := CompiledPolicy{PolicyID: definition.ID, Version: definition.Version}
	for i := range bindings {
		binding := &bindings[i]
		if !binding.Active {
			continue
		}
		if _, ok := pipelines[binding.PipelineName]; !ok {
			return CompiledPolicy{}, fmt.Errorf("binding %d selects unknown pipeline %q", binding.ID, binding.PipelineName)
		}
		if binding.BindingKind != PipelineBindingKindConditional {
			continue
		}
		checksum, err := compilePredicate(binding.Predicate, binding.PredicateChecksum)
		if err != nil {
			return CompiledPolicy{}, fmt.Errorf("binding %d: %w", binding.ID, err)
		}
		binding.PredicateChecksum = checksum
		compiled.BindingChecksums = append(compiled.BindingChecksums, checksum)
		if binding.LegacyRule != nil {
			legacy, legacyChecksum, err := LegacyRulePredicateDocument(*binding.LegacyRule)
			if err != nil || legacyChecksum != checksum || !canonicalPredicatesEqual(legacy, binding.Predicate) {
				return CompiledPolicy{}, fmt.Errorf("binding %d legacy adapter parity mismatch", binding.ID)
			}
		}
	}
	for i := range gates {
		gate := &gates[i]
		if !gate.Active {
			continue
		}
		if !processors[normalizeRuntimeName(gate.TargetProcessor)] {
			return CompiledPolicy{}, fmt.Errorf("gate %d targets unknown processor %q", gate.ID, gate.TargetProcessor)
		}
		if !validGateEffect(gate.Effect) {
			return CompiledPolicy{}, fmt.Errorf("gate %d has invalid effect %q", gate.ID, gate.Effect)
		}
		checksum, err := compilePredicate(gate.Predicate, gate.PredicateChecksum)
		if err != nil {
			return CompiledPolicy{}, fmt.Errorf("gate %d: %w", gate.ID, err)
		}
		gate.PredicateChecksum = checksum
		analysis := semrules.Analyze(gate.Predicate)
		storedFacets := sortedUniqueNonEmpty(gate.RequiredFacets)
		if len(storedFacets) > 0 && !sameStringSlice(storedFacets, analysis.RequiredDocumentFacets) {
			return CompiledPolicy{}, fmt.Errorf("gate %d required facets do not match predicate analysis", gate.ID)
		}
		gate.RequiredFacets = append([]string(nil), analysis.RequiredDocumentFacets...)
		compiled.GateChecksums = append(compiled.GateChecksums, checksum)
		if gate.Effect == GateEffectDefer && len(analysis.RequiredPaths) == 0 {
			return CompiledPolicy{}, fmt.Errorf("gate %d defer effect requires dependency facts", gate.ID)
		}
	}
	for i := 0; i < len(bindings); i++ {
		for j := i + 1; j < len(bindings); j++ {
			left, right := bindings[i], bindings[j]
			if !left.Active || !right.Active || left.BindingKind != PipelineBindingKindConditional || right.BindingKind != PipelineBindingKindConditional || !sameBindingRank(left, right) {
				continue
			}
			overlap := semrules.AnalyzeOverlap(left.Predicate, right.Predicate)
			if !overlap.Analyzable {
				compiled.Warnings = append(compiled.Warnings, "runtime_conflict_check_required")
			} else if overlap.MayOverlap && left.PipelineName != right.PipelineName {
				return CompiledPolicy{}, fmt.Errorf("overlapping same-rank bindings %d and %d select different pipelines", left.ID, right.ID)
			}
		}
	}
	validSubjects := map[string]string{}
	baseline := pipelines[DefaultProductionPipelineName]
	for _, binding := range bindings {
		if binding.Active && binding.BindingKind == PipelineBindingKindConditional {
			validSubjects[policySubjectKey("conditional_binding", binding.ID)] = ConditionalBindingSubjectChecksum(definition.Version, binding, pipelines[binding.PipelineName], baseline)
		}
	}
	for _, gate := range gates {
		if gate.Active {
			validSubjects[policySubjectKey("processor_rule", gate.ID)] = ProcessorGateSubjectChecksum(definition.Version, gate)
		}
	}
	for _, reference := range definition.ClearanceReferences {
		if checksum, ok := validSubjects[policySubjectKey(reference.SubjectKind, reference.SubjectID)]; !ok || checksum != strings.TrimSpace(reference.SubjectChecksum) {
			return CompiledPolicy{}, fmt.Errorf("clearance reference %s/%d does not match compiled subject", reference.SubjectKind, reference.SubjectID)
		}
	}
	compiled.BindingChecksums = sortedUniqueNonEmpty(compiled.BindingChecksums)
	compiled.GateChecksums = sortedUniqueNonEmpty(compiled.GateChecksums)
	compiled.Warnings = sortedUniqueNonEmpty(compiled.Warnings)
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].ID < bindings[j].ID })
	sort.Slice(gates, func(i, j int) bool { return gates[i].ID < gates[j].ID })
	pipelineList := append([]ProductionPipelineSpec(nil), definition.Pipelines...)
	sort.Slice(pipelineList, func(i, j int) bool { return pipelineList[i].Name < pipelineList[j].Name })
	raw, err := json.Marshal(struct {
		ID        int64
		Version   int
		Pipelines []ProductionPipelineSpec
		Bindings  []PipelineBinding
		Gates     []PipelineGate
	}{definition.ID, definition.Version, pipelineList, bindings, gates})
	if err != nil {
		return CompiledPolicy{}, err
	}
	digest := sha256.Sum256(raw)
	compiled.Checksum = "sha256:" + hex.EncodeToString(digest[:])
	return compiled, nil
}

func compilePredicate(document semrules.Document, stored string) (string, error) {
	if err := semrules.Validate(document); err != nil {
		return "", err
	}
	_, checksum, err := semrules.Canonicalize(document)
	if err != nil {
		return "", err
	}
	if stored = strings.TrimSpace(stored); stored != "" && stored != checksum {
		return "", fmt.Errorf("predicate checksum mismatch: stored %s canonical %s", stored, checksum)
	}
	return checksum, nil
}

func canonicalPredicatesEqual(left, right semrules.Document) bool {
	l, _, le := semrules.Canonicalize(left)
	r, _, re := semrules.Canonicalize(right)
	return le == nil && re == nil && string(l) == string(r)
}

func ProcessorGateSubjectChecksum(policyVersion int, gate PipelineGate) string {
	return hashPolicySubject([]any{policyVersion, normalizeRuntimeName(gate.TargetProcessor), gate.Effect, gate.PredicateChecksum})
}

func ConditionalBindingSubjectChecksum(policyVersion int, binding PipelineBinding, selected, baseline ProductionPipelineSpec) string {
	selectedChecksum, _ := productionPipelineSpecChecksum(selected)
	baselineChecksum, _ := productionPipelineSpecChecksum(baseline)
	return hashPolicySubject([]any{policyVersion, binding.PipelineName, binding.PredicateChecksum, selectedChecksum, baselineChecksum})
}

func hashPolicySubject(value any) string {
	raw, _ := json.Marshal(value)
	digest := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func policySubjectKey(kind string, id int64) string {
	return fmt.Sprintf("%s/%d", strings.TrimSpace(kind), id)
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

