package docreviews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/searchengine"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// Candidate verdicts (openspec compliance-candidate-verdicts): every candidate of an assessed
// activity gets one verdict and a summary of how it applies to the activity. The model proposes
// them in the assessment call; code checks them against the model's own requirement assessments
// inside the call's correction retries, then downgrades strong verdicts that the final, verified
// requirement results no longer support. Code never raises a verdict.
const (
	verdictCompliant    = "compliant"
	verdictCompatible   = "compatible"
	verdictNotCompliant = "not_compliant"
	verdictUndetermined = "undetermined"
	verdictUnqualified  = "unqualified"

	// complianceCandidateTextMax caps each candidate's indexed text in the assessment input.
	complianceCandidateTextMax  = 2000
	complianceStructuredRetries = 2
)

// complianceVerdictOrder is the display order: actionable verdicts first, droppable last.
var complianceVerdictOrder = []string{verdictNotCompliant, verdictCompatible, verdictCompliant, verdictUndetermined, verdictUnqualified}

type complianceCandidateInput struct {
	ArtifactKey          string   `json:"artifact_key"`
	Type                 string   `json:"type"`
	RecordID             int64    `json:"record_id"`
	Title                string   `json:"title,omitempty"`
	Text                 string   `json:"text"`
	Truncated            bool     `json:"truncated,omitempty"`
	LinkedRequirementIDs []string `json:"linked_requirement_ids"`
}

type complianceCandidateVerdict struct {
	ArtifactKey    string   `json:"artifact_key"`
	Verdict        string   `json:"verdict"`
	RequirementIDs []string `json:"requirement_ids"`
	Summary        string   `json:"summary"`
	DraftVerdict   string   `json:"draft_verdict,omitempty"`
	Verification   string   `json:"verification,omitempty"`
	// Note says why a drafted verdict became undetermined (verifier reason or downgrade cause).
	Note string `json:"note,omitempty"`
}

type complianceCandidateCheck struct {
	ArtifactKey string `json:"artifact_key"`
	Valid       bool   `json:"valid"`
	Reason      string `json:"reason"`
}

// complianceCandidateInputs builds the assessment's candidate list: every related artifact with its
// indexed text (one searchengine.Load per activity) and the requirements linked to it.
func (r *complianceReviewer) complianceCandidateInputs(ctx context.Context, artifacts []complianceArtifact, reqs []complianceRequirement) ([]complianceCandidateInput, error) {
	links := complianceRequirementLinks(reqs)
	texts := map[string]searchengine.Result{}
	if r.loadArtifacts != nil && len(artifacts) > 0 {
		refs := make([]searchengine.ArtifactRef, 0, len(artifacts))
		for _, a := range artifacts {
			refs = append(refs, searchengine.ArtifactRef{Type: a.Type, ID: a.ID})
		}
		loaded, err := r.loadArtifacts(ctx, refs)
		if err != nil {
			return nil, fmt.Errorf("compliance: load candidate text: %w", err)
		}
		for _, res := range loaded {
			texts[res.ArtifactType+"/"+res.ArtifactID] = res
		}
	}
	out := make([]complianceCandidateInput, 0, len(artifacts))
	for _, a := range artifacts {
		res := texts[a.key()]
		title := res.SourceTitle
		if title == "" {
			title = res.SourceFilename
		}
		text := strings.TrimSpace(res.Snippet)
		truncated := false
		if runes := []rune(text); len(runes) > complianceCandidateTextMax {
			text, truncated = string(runes[:complianceCandidateTextMax]), true
		}
		linked := links[a.key()]
		if linked == nil {
			linked = []string{}
		}
		out = append(out, complianceCandidateInput{ArtifactKey: a.key(), Type: a.Type, RecordID: a.RecordID, Title: title, Text: text, Truncated: truncated, LinkedRequirementIDs: linked})
	}
	return out, nil
}

// complianceRequirementLinks maps each artifact key to the IDs of the requirements it expresses.
func complianceRequirementLinks(reqs []complianceRequirement) map[string][]string {
	links := map[string][]string{}
	for _, req := range reqs {
		for _, key := range req.ArtifactKeys {
			links[key] = append(links[key], req.ID)
		}
	}
	return links
}

// complianceVerdictBasis evaluates the strong-verdict rules over a candidate's linked requirements.
// Required means binding "required"; recommendations never decide compliance.
//   - contradicted: a linked required requirement is applicable and contradicted.
//   - compliant: at least one linked required requirement is applicable and supported, and every
//     other linked required requirement is supported (and applicable) or not applicable.
//   - applicable: a linked required requirement is applicable (the candidate can judge the activity).
type complianceVerdictBasis struct {
	contradicted, compliant, applicable bool
	contradictedBy                      string
}

func complianceBasis(linked []string, reqs map[string]complianceRequirement, results map[string]complianceAssessment) complianceVerdictBasis {
	var b complianceVerdictBasis
	supported, blocked := false, false
	for _, id := range linked {
		req, res := reqs[id], results[id]
		if req.Binding != "required" {
			continue
		}
		switch {
		case res.Applicability == "applicable" && res.Status == "contradicted":
			b.contradicted, b.applicable = true, true
			if b.contradictedBy == "" {
				b.contradictedBy = id
			}
			blocked = true
		case res.Applicability == "applicable" && res.Status == "supported":
			b.applicable, supported = true, true
		case res.Applicability == "applicable":
			b.applicable, blocked = true, true
		case res.Applicability != "not_applicable":
			blocked = true
		}
	}
	b.compliant = supported && !blocked
	return b
}

func complianceIndex(reqs []complianceRequirement, results []complianceAssessment) (map[string]complianceRequirement, map[string]complianceAssessment) {
	byReq := make(map[string]complianceRequirement, len(reqs))
	for _, req := range reqs {
		byReq[req.ID] = req
	}
	byRes := make(map[string]complianceAssessment, len(results))
	for _, res := range results {
		byRes[res.RequirementID] = res
	}
	return byReq, byRes
}

// checkComplianceCandidateDrafts checks the drafted verdicts against the candidate list and the
// model's own requirement assessments. Every problem names the candidate and the field and says what
// to return, and all are reported together so one retry can fix them.
func checkComplianceCandidateDrafts(verdicts []complianceCandidateVerdict, cands []complianceCandidateInput, reqs []complianceRequirement, drafts []complianceAssessment) error {
	byReq, byRes := complianceIndex(reqs, drafts)
	known := make(map[string]complianceCandidateInput, len(cands))
	keys := make([]string, 0, len(cands))
	for _, c := range cands {
		known[c.ArtifactKey] = c
		keys = append(keys, c.ArtifactKey)
	}
	allowed := strings.Join(complianceVerdictOrder, ", ")
	var problems []string
	seen := map[string]bool{}
	for i, v := range verdicts {
		c, ok := known[v.ArtifactKey]
		if !ok {
			problems = append(problems, fmt.Sprintf("Candidate entry %d has artifact_key %q, which is not a supplied candidate. Use one of: %s.", i+1, v.ArtifactKey, strings.Join(keys, ", ")))
			continue
		}
		if seen[v.ArtifactKey] {
			problems = append(problems, fmt.Sprintf("Candidate %s appears more than once. Return exactly one entry for it.", v.ArtifactKey))
			continue
		}
		seen[v.ArtifactKey] = true
		if !complianceOneOf(v.Verdict, complianceVerdictOrder...) {
			problems = append(problems, fmt.Sprintf("Candidate %s has verdict %q. Use one of: %s.", v.ArtifactKey, v.Verdict, allowed))
			continue
		}
		if strings.TrimSpace(v.Summary) == "" {
			problems = append(problems, fmt.Sprintf("Candidate %s has an empty summary. Explain how the candidate applies to the activity.", v.ArtifactKey))
		}
		linkedSet := map[string]bool{}
		for _, id := range c.LinkedRequirementIDs {
			linkedSet[id] = true
		}
		for _, id := range v.RequirementIDs {
			if !linkedSet[id] {
				problems = append(problems, fmt.Sprintf("Candidate %s cites requirement_id %q, which is not linked to it. Use only ids from its linked_requirement_ids: [%s].", v.ArtifactKey, id, strings.Join(c.LinkedRequirementIDs, ", ")))
			}
		}
		if len(c.LinkedRequirementIDs) == 0 {
			if !complianceOneOf(v.Verdict, verdictUnqualified, verdictUndetermined) {
				problems = append(problems, fmt.Sprintf("Candidate %s has verdict %s but no linked requirement. Use unqualified, or undetermined when its text states a requirement on this activity.", v.ArtifactKey, v.Verdict))
			}
			continue
		}
		basis := complianceBasis(c.LinkedRequirementIDs, byReq, byRes)
		switch {
		case basis.contradicted && v.Verdict != verdictNotCompliant:
			problems = append(problems, fmt.Sprintf("Candidate %s has verdict %s, but its required requirement %s is assessed applicable and contradicted. Return not_compliant, or correct the assessment of %s.", v.ArtifactKey, v.Verdict, basis.contradictedBy, basis.contradictedBy))
		case v.Verdict == verdictNotCompliant && !basis.contradicted:
			problems = append(problems, fmt.Sprintf("Candidate %s has verdict not_compliant, but none of its linked required requirements [%s] is assessed applicable and contradicted. Return compatible, undetermined or unqualified, or correct the assessment.", v.ArtifactKey, strings.Join(c.LinkedRequirementIDs, ", ")))
		case v.Verdict == verdictCompliant && !basis.compliant:
			problems = append(problems, fmt.Sprintf("Candidate %s has verdict compliant, but its linked required requirements [%s] are not all assessed applicable and supported (or not_applicable), with at least one supported. Return compatible or undetermined, or correct the assessment.", v.ArtifactKey, strings.Join(c.LinkedRequirementIDs, ", ")))
		case v.Verdict == verdictUnqualified && basis.applicable:
			problems = append(problems, fmt.Sprintf("Candidate %s has verdict unqualified, but a linked required requirement is assessed applicable, so the candidate can judge the activity. Return compliant, compatible, not_compliant or undetermined.", v.ArtifactKey))
		}
	}
	for _, key := range keys {
		if !seen[key] {
			problems = append(problems, fmt.Sprintf("Candidate %s has no entry in candidates. Return one entry with its verdict and summary.", key))
		}
	}
	if len(problems) > 0 {
		return errors.New("compliance candidate verdicts: " + strings.Join(problems, " "))
	}
	return nil
}

// checkComplianceCandidateChecks requires exactly one verifier check, with a reason, per candidate.
func checkComplianceCandidateChecks(checks []complianceCandidateCheck, cands []complianceCandidateInput) error {
	known := map[string]bool{}
	keys := make([]string, 0, len(cands))
	for _, c := range cands {
		known[c.ArtifactKey] = true
		keys = append(keys, c.ArtifactKey)
	}
	var problems []string
	seen := map[string]bool{}
	for i, check := range checks {
		switch {
		case !known[check.ArtifactKey]:
			problems = append(problems, fmt.Sprintf("candidate_checks entry %d has artifact_key %q, which is not a supplied candidate. Use one of: %s.", i+1, check.ArtifactKey, strings.Join(keys, ", ")))
		case seen[check.ArtifactKey]:
			problems = append(problems, fmt.Sprintf("Candidate %s has more than one check. Return exactly one.", check.ArtifactKey))
		case strings.TrimSpace(check.Reason) == "":
			problems = append(problems, fmt.Sprintf("The check for candidate %s has an empty reason. Give a concise evidence-based reason.", check.ArtifactKey))
		}
		seen[check.ArtifactKey] = true
	}
	for _, key := range keys {
		if !seen[key] {
			problems = append(problems, fmt.Sprintf("Candidate %s has no entry in candidate_checks. Return one check for it.", key))
		}
	}
	if len(problems) > 0 {
		return errors.New("compliance candidate checks: " + strings.Join(problems, " "))
	}
	return nil
}

// finalizeComplianceVerdicts applies the verifier's candidate checks, then re-evaluates the strong
// verdicts on the final (validated and verified) requirement results. Anything that no longer holds
// becomes undetermined with the cause as its note.
func finalizeComplianceVerdicts(verdicts []complianceCandidateVerdict, cands []complianceCandidateInput, reqs []complianceRequirement, final []complianceAssessment, checks []complianceCandidateCheck) {
	byReq, byRes := complianceIndex(reqs, final)
	linked := map[string][]string{}
	for _, c := range cands {
		linked[c.ArtifactKey] = c.LinkedRequirementIDs
	}
	byCheck := map[string]complianceCandidateCheck{}
	for _, check := range checks {
		byCheck[check.ArtifactKey] = check
	}
	for i := range verdicts {
		v := &verdicts[i]
		v.DraftVerdict = v.Verdict
		check := byCheck[v.ArtifactKey]
		v.Verification = check.Reason
		if !check.Valid {
			v.Verdict, v.Note = verdictUndetermined, check.Reason
			continue
		}
		basis := complianceBasis(linked[v.ArtifactKey], byReq, byRes)
		if (v.Verdict == verdictNotCompliant && !basis.contradicted) || (v.Verdict == verdictCompliant && !basis.compliant) {
			v.Verdict, v.Note = verdictUndetermined, complianceDowngradeCause(v.Verdict, linked[v.ArtifactKey], byReq, byRes)
		}
	}
}

// complianceDowngradeCause names each linked required requirement whose final result no longer
// supports the drafted strong verdict, with the reason its result changed.
func complianceDowngradeCause(verdict string, linked []string, reqs map[string]complianceRequirement, results map[string]complianceAssessment) string {
	var parts []string
	for _, id := range linked {
		req, res := reqs[id], results[id]
		if req.Binding != "required" || res.Applicability == "not_applicable" {
			continue
		}
		if verdict == verdictCompliant && res.Applicability == "applicable" && res.Status == "supported" {
			continue
		}
		cause := res.ResultNote
		if cause == "" {
			cause = res.NumericCheck
		}
		if cause == "" {
			cause = res.Rationale
		}
		parts = append(parts, fmt.Sprintf("%s is %s (applicability %s): %s", id, res.Status, res.Applicability, cause))
	}
	msg := fmt.Sprintf("Drafted %s, but the final requirement results do not support it.", verdict)
	if len(parts) == 0 {
		return msg
	}
	return msg + " " + strings.Join(parts, "; ")
}

// orderComplianceVerdicts returns the verdicts sorted for display, keeping candidate order within
// one verdict.
func orderComplianceVerdicts(verdicts []complianceCandidateVerdict) []complianceCandidateVerdict {
	out := make([]complianceCandidateVerdict, 0, len(verdicts))
	for _, verdict := range complianceVerdictOrder {
		for _, v := range verdicts {
			if v.Verdict == verdict {
				out = append(out, v)
			}
		}
	}
	return out
}

// complianceVerdictRows builds the per-candidate related_artifacts entries and the row text section.
// Verdict lines start with the verdict, so compliance-score's "- [type id] ... record N" pattern
// does not read them as reported passages.
func complianceVerdictRows(verdicts []complianceCandidateVerdict, cands []complianceCandidateInput) ([]RelatedArtifactAnalysis, string) {
	byKey := map[string]complianceCandidateInput{}
	for _, c := range cands {
		byKey[c.ArtifactKey] = c
	}
	ordered := orderComplianceVerdicts(verdicts)
	counts := map[string]int{}
	for _, v := range ordered {
		counts[v.Verdict]++
	}
	var b strings.Builder
	var countParts []string
	for _, verdict := range complianceVerdictOrder {
		if counts[verdict] > 0 {
			countParts = append(countParts, fmt.Sprintf("%d %s", counts[verdict], verdict))
		}
	}
	fmt.Fprintf(&b, "Candidate verdicts: %s\n", strings.Join(countParts, ", "))
	related := make([]RelatedArtifactAnalysis, 0, len(ordered))
	for _, v := range ordered {
		c := byKey[v.ArtifactKey]
		fmt.Fprintf(&b, "%s [%s %s] record %d: %s\n", v.Verdict, c.Type, strings.TrimPrefix(v.ArtifactKey, c.Type+"/"), c.RecordID, v.Summary)
		if v.Note != "" {
			fmt.Fprintf(&b, "  Note: %s\n", v.Note)
		}
		related = append(related, RelatedArtifactAnalysis{
			RelatedArtifactID: strings.TrimPrefix(v.ArtifactKey, c.Type+"/"),
			RelatedRecordID:   c.RecordID,
			Relationship:      v.Verdict,
			Summary:           v.Summary,
			Note:              v.Note,
		})
	}
	b.WriteString("\n")
	return related, b.String()
}

// complianceAssessmentSchema is the structured-output schema of the assessment stage. The
// requirement assessments keep their existing post-call validation; the schema only fixes their shape.
func complianceAssessmentSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []any{"assessments", "candidates"},
		"properties": map[string]any{
			"assessments": map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []any{"requirement_id"}}},
			"candidates": map[string]any{"type": "array", "items": map[string]any{
				"type":     "object",
				"required": []any{"artifact_key", "verdict", "requirement_ids", "summary"},
				"properties": map[string]any{
					"artifact_key":    map[string]any{"type": "string"},
					"verdict":         map[string]any{"type": "string", "enum": []any{verdictCompliant, verdictCompatible, verdictNotCompliant, verdictUndetermined, verdictUnqualified}},
					"requirement_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"summary":         map[string]any{"type": "string"},
				},
			}},
		},
	}
}

func complianceVerifierSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []any{"checks", "candidate_checks"},
		"properties": map[string]any{
			"checks": map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []any{"requirement_id", "valid", "reason"}}},
			"candidate_checks": map[string]any{"type": "array", "items": map[string]any{
				"type":     "object",
				"required": []any{"artifact_key", "valid", "reason"},
				"properties": map[string]any{
					"artifact_key": map[string]any{"type": "string"},
					"valid":        map[string]any{"type": "boolean"},
					"reason":       map[string]any{"type": "string"},
				},
			}},
		},
	}
}

// structuredAssessmentCall is assessmentCall with a schema and a CheckParsed hook: a client that
// supports correction retries gets the check inside its retry loop (the same model corrects the
// answer); any other client runs the check once after the call and the reviewer aborts on failure.
func (r *complianceReviewer) structuredAssessmentCall(ctx context.Context, stage string, cfg ReviewerConfig, input any, schema map[string]any, check func(map[string]any) error) (map[string]any, error) {
	structured, ok := r.client.(docprocessing.LLMStructuredJSONExtractor)
	if !ok {
		payload, err := r.assessmentCall(ctx, stage, cfg, input)
		if err != nil {
			return nil, err
		}
		if err = check(payload); err != nil {
			return nil, err
		}
		return payload, nil
	}
	text, resolved, err := r.assessmentPrompt(stage)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	contract := llmclients.StructuredOutputContract{Name: "chenweb_review_compliance_" + stage, Schema: schemaJSON, AllowRepair: true, MaxRetries: complianceStructuredRetries, CheckParsed: check}
	result, err := structured.ExtractStructuredJSON(ctx, newDocReviewLLMJSONInput(ctx, resolved, text, cfg.ModelName, string(b), "review_compliance_"+stage, "20261121-417"), contract)
	if err != nil {
		return nil, err
	}
	return result.Parsed, nil
}

// complianceDecodeAssessment reads the requirement assessments and candidate verdicts of an assessment payload.
func complianceDecodeAssessment(payload map[string]any) ([]complianceAssessment, []complianceCandidateVerdict, error) {
	assessments, err := complianceArray[complianceAssessment](payload, "assessments")
	if err != nil {
		return nil, nil, err
	}
	verdicts, err := complianceArray[complianceCandidateVerdict](payload, "candidates")
	if err != nil {
		return nil, nil, err
	}
	return assessments, verdicts, nil
}
