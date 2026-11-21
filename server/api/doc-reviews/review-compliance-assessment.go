package docreviews

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

func complianceAssessmentPromptRefs() map[string]string {
	return map[string]string{
		"requirements": "prompt-review-compliance-requirements-v2.md",
		"screen":       "prompt-review-compliance-evidence-v1.md",
		"assess":       "prompt-review-compliance-assessment-v2.md",
		"verify":       "prompt-review-compliance-verifier-v2.md",
	}
}

type complianceRequirement struct {
	ID            string               `json:"requirement_id"`
	Text          string               `json:"text"`
	Kind          string               `json:"kind"`
	Binding       string               `json:"binding"`
	ArtifactKeys  []string             `json:"artifact_keys"` // every candidate of the source that expresses it
	Source        complianceEvidence   `json:"source"`
	Conditions    string               `json:"conditions"`
	ScopeEvidence []complianceEvidence `json:"scope_evidence"`
}
type complianceScreening struct {
	RequirementID        string               `json:"requirement_id"`
	Evidence             []complianceEvidence `json:"evidence"`
	UnresolvedReferences []string             `json:"unresolved_references"`
}
type complianceAssessment struct {
	RequirementID       string                          `json:"requirement_id"`
	Applicability       string                          `json:"applicability"`
	ApplicabilityReason string                          `json:"applicability_reason"`
	Status              string                          `json:"status"`
	Rationale           string                          `json:"rationale"`
	Suggestion          string                          `json:"suggestion"`
	Evidence            []complianceEvidence            `json:"evidence"`
	Numeric             *complianceNumeric              `json:"numeric,omitempty"`
	NumericCheck        string                          `json:"numeric_check,omitempty"`
	Verification        string                          `json:"verification,omitempty"`
	References          []complianceReferenceResolution `json:"reference_resolutions,omitempty"`
	// ResultNote says why the server or the verifier changed the drafted status.
	ResultNote string `json:"result_note,omitempty"`
}
type complianceLineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}
type complianceRetrievalTrace struct {
	Query                string                `json:"query"`
	ArtifactType         string                `json:"artifact_type"`
	Limit                int                   `json:"limit"`
	PotentiallyTruncated bool                  `json:"potentially_truncated"`
	Results              []searchengine.Result `json:"results"`
}
type complianceCoverage struct {
	ReviewedRecordID   int64                      `json:"reviewed_record_id"`
	EvidenceBoundary   string                     `json:"evidence_boundary"`
	TotalLines         int                        `json:"total_lines"`
	ScreenedLines      int                        `json:"screened_lines"`
	EvidenceComplete   bool                       `json:"evidence_complete"`
	SourceComplete     bool                       `json:"source_complete"`
	SourceRecords      int                        `json:"source_records"`
	Issues             []string                   `json:"issues"`
	ScreenedRanges     []complianceLineRange      `json:"screened_ranges"`
	UnexaminedRanges   []complianceLineRange      `json:"unexamined_ranges"`
	DependencySearches []complianceRetrievalTrace `json:"dependency_searches,omitempty"`
}
type complianceVerification struct {
	RequirementID string `json:"requirement_id"`
	Valid         bool   `json:"valid"`
	Reason        string `json:"reason"`
}

func complianceArray[T any](payload map[string]any, key string) ([]T, error) {
	value, ok := payload[key]
	if !ok || value == nil {
		return nil, fmt.Errorf("compliance: model response missing %s array", key)
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || b[0] != '[' {
		return nil, fmt.Errorf("compliance: %s must be an array", key)
	}
	var out []T
	if err = json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("compliance: invalid %s: %w", key, err)
	}
	return out, nil
}

func (r *complianceReviewer) assessmentCall(ctx context.Context, stage string, cfg ReviewerConfig, input any) (map[string]any, error) {
	ref := r.assessmentPrompts[stage]
	if ref == "" || r.loadPrompt == nil {
		return nil, fmt.Errorf("compliance: %s prompt not configured", stage)
	}
	text, resolved, err := r.loadPrompt(ref)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	return r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, resolved, text, cfg.ModelName, string(b), "review_compliance_"+stage, "20261114-614"))
}

// assessActivity keeps one row per activity for historical scorer compatibility. Detailed,
// separately anchored obligations are retained in artifact_fields, and readable in the row.
func (r *complianceReviewer) assessActivity(ctx context.Context, recordID int64, a complianceActivity, cfg ReviewerConfig, f ReviewFinding) (ReviewFinding, error) {
	metadata := map[string]any{}
	if len(f.ArtifactFields) > 0 {
		if err := json.Unmarshal(f.ArtifactFields, &metadata); err != nil {
			return f, err
		}
	}
	metadata["assessment_date"] = r.assessmentDate
	metadata["model"] = cfg.ModelName
	metadata["prompts"] = r.assessmentPrompts
	var artifacts []complianceArtifact
	if err := json.Unmarshal(f.RelatedArtifactFields, &artifacts); err != nil {
		return f, err
	}
	initialCandidates := len(artifacts)
	doc, err := r.document(ctx, recordID)
	if err != nil {
		return f, err
	}
	docs := map[int64]complianceDocument{recordID: doc}
	coverage := complianceCoverage{ReviewedRecordID: recordID, EvidenceBoundary: "reviewed document only", TotalLines: len(doc.Lines), EvidenceComplete: doc.Complete, SourceComplete: true, Issues: append([]string(nil), doc.Issues...)}
	requirements, err := r.discoverRequirements(ctx, a, cfg, &artifacts, docs, &coverage)
	if err != nil {
		return f, err
	}
	f.RelatedArtifactFields, _ = json.Marshal(artifacts)
	for _, art := range artifacts[initialCandidates:] {
		f.Description += fmt.Sprintf("- [%s %s] record %d dependency discovery\n", art.Type, art.ID, art.RecordID)
	}
	candidates, err := r.complianceCandidateInputs(ctx, artifacts, requirements)
	if err != nil {
		return f, err
	}
	// Activities whose candidates yield no requirement still get a verdict per candidate; there is
	// nothing to screen the document for.
	var screenings []complianceScreening
	if len(requirements) == 0 {
		coverage.EvidenceComplete = false
	} else if screenings, err = r.screenDocument(ctx, a, cfg, requirements, doc, &coverage); err != nil {
		return f, err
	}
	payload, err := r.structuredAssessmentCall(ctx, "assess", cfg, map[string]any{
		"activity": a, "document_context": doc.Context, "assessment_date": r.assessmentDate, "requirements": requirements, "screenings": screenings, "coverage": coverage, "candidates": candidates,
	}, complianceAssessmentSchema(), func(parsed map[string]any) error {
		drafts, verdicts, derr := complianceDecodeAssessment(parsed)
		if derr != nil {
			return derr
		}
		return checkComplianceCandidateDrafts(verdicts, candidates, requirements, drafts)
	})
	if err != nil {
		return f, err
	}
	assessments, verdicts, err := complianceDecodeAssessment(payload)
	if err != nil {
		return f, err
	}
	if err = validateComplianceAssessments(assessments, requirements, docs, coverage); err != nil {
		return f, err
	}
	if err = validateComplianceReferences(assessments, screenings, docs, recordID); err != nil {
		return f, err
	}
	// A separate semantic check is necessary: matching quote bytes does not prove entailment.
	verified, err := r.structuredAssessmentCall(ctx, "verify", cfg, map[string]any{"activity": a, "assessment_date": r.assessmentDate, "requirements": requirements, "assessments": assessments, "screenings": screenings, "coverage": coverage, "candidates": candidates, "candidate_verdicts": verdicts},
		complianceVerifierSchema(), func(parsed map[string]any) error {
			candidateChecks, derr := complianceArray[complianceCandidateCheck](parsed, "candidate_checks")
			if derr != nil {
				return derr
			}
			return checkComplianceCandidateChecks(candidateChecks, candidates)
		})
	if err != nil {
		return f, err
	}
	checks, err := complianceArray[complianceVerification](verified, "checks")
	if err != nil {
		return f, err
	}
	candidateChecks, err := complianceArray[complianceCandidateCheck](verified, "candidate_checks")
	if err != nil {
		return f, err
	}
	if len(checks) != len(assessments) {
		return f, fmt.Errorf("compliance: verifier omitted requirements")
	}
	byID := map[string]complianceVerification{}
	for _, check := range checks {
		if _, exists := byID[check.RequirementID]; exists || check.Reason == "" {
			return f, fmt.Errorf("compliance: invalid verifier check")
		}
		byID[check.RequirementID] = check
	}
	var b strings.Builder
	b.WriteString(strings.ReplaceAll(f.Description, "Candidates are search results; applicability and requirement comparison are not assessed.", "Candidates are search results; the candidate verdicts above and the requirement assessments below record applicability and document evidence."))
	if len(requirements) == 0 {
		b.WriteString("\nSource analysis found no assessable governing requirement. Retrieved candidates are not a compliance pass.\n")
	}
	for _, req := range requirements {
		fmt.Fprintf(&b, "\nRequirement %s [%s]: %s\nSource: record %d lines %d-%d: %s\n", req.ID, req.Binding, req.Text, req.Source.RecordID, req.Source.LineStart, req.Source.LineEnd, req.Source.Quote)
	}
	if len(requirements) > 0 {
		b.WriteString("\nRequirement assessments (documentary evidence only):\n")
	}
	for i := range assessments {
		result := &assessments[i]
		check, ok := byID[result.RequirementID]
		if !ok {
			return f, fmt.Errorf("compliance: verifier returned unknown requirement")
		}
		result.Verification = check.Reason
		if !check.Valid {
			result.Status = "insufficient_evidence"
			result.Applicability = "unknown"
			result.Suggestion = ""
			result.Rationale = "The draft conclusion is unresolved: " + check.Reason
			result.ApplicabilityReason = check.Reason
			result.ResultNote = "Verification rejected the draft: " + check.Reason
		}
		if check.Valid && result.Applicability == "applicable" && complianceOneOf(result.Status, "contradicted", "not_documented") {
			f.Severity = "medium"
		}
		fmt.Fprintf(&b, "- %s: applicability=%s; evidence=%s. %s %s\n", result.RequirementID, result.Applicability, result.Status, result.ApplicabilityReason, result.Rationale)
		if result.NumericCheck != "" {
			fmt.Fprintf(&b, "  Numeric verification: %s\n", result.NumericCheck)
		}
		fmt.Fprintf(&b, "  Verification: %s\n", result.Verification)
		for _, e := range result.Evidence {
			fmt.Fprintf(&b, "  Evidence: record %d lines %d-%d: %s\n", e.RecordID, e.LineStart, e.LineEnd, e.Quote)
		}
		if result.Suggestion != "" {
			fmt.Fprintf(&b, "  Suggested correction: %s\n", result.Suggestion)
		}
	}
	fmt.Fprintf(&b, "\nEvidence coverage: %d/%d canonical lines screened; complete=%t; source context complete=%t; boundary=%s.\n", coverage.ScreenedLines, coverage.TotalLines, coverage.EvidenceComplete, coverage.SourceComplete, coverage.EvidenceBoundary)
	for _, issue := range coverage.Issues {
		fmt.Fprintf(&b, "- Coverage limit: %s\n", issue)
	}
	finalizeComplianceVerdicts(verdicts, candidates, requirements, assessments, candidateChecks)
	related, verdictText := complianceVerdictRows(verdicts, candidates)
	f.RelatedArtifacts = related
	f.Description = verdictText + b.String()
	if len(requirements) > 0 {
		f.FindingType = "compliance_assessment"
		f.ResultKind = "compliance_assessment"
		label := a.Activity
		if a.DocumentLevel {
			label = "document: " + a.Object
		}
		f.Title = "Compliance assessment: " + label
	}
	metadata["version"] = 2
	metadata["activity"] = a
	metadata["requirements"] = requirements
	metadata["assessments"] = assessments
	metadata["coverage"] = coverage
	metadata["sources"] = complianceManifests(docs)
	metadata["screenings"] = screenings
	metadata["candidate_verdicts"] = orderComplianceVerdicts(verdicts)
	f.ArtifactFields, _ = json.Marshal(metadata)
	return f, nil
}

func complianceManifests(docs map[int64]complianceDocument) map[int64]any {
	out := map[int64]any{}
	for id, d := range docs {
		out[id] = map[string]any{"sha256": d.Hash, "context": d.Context, "lines": len(d.Lines), "complete": d.Complete, "issues": d.Issues}
	}
	return out
}

func (r *complianceReviewer) discoverRequirements(ctx context.Context, a complianceActivity, cfg ReviewerConfig, artifacts *[]complianceArtifact, docs map[int64]complianceDocument, coverage *complianceCoverage) ([]complianceRequirement, error) {
	maxSources := r.maxSourceRecords
	if maxSources <= 0 {
		maxSources = 8
	}
	processed := map[int64]bool{}
	seenArtifact := map[string]bool{}
	for _, art := range *artifacts {
		seenArtifact[art.key()] = true
	}
	var out []complianceRequirement
	dedup := map[string]bool{}
	for pos := 0; pos < len(*artifacts); pos++ {
		art := (*artifacts)[pos]
		if processed[art.RecordID] {
			continue
		}
		if len(processed) >= maxSources {
			coverage.SourceComplete = false
			coverage.Issues = append(coverage.Issues, "Source-record expansion budget exhausted.")
			break
		}
		processed[art.RecordID] = true
		source, err := r.document(ctx, art.RecordID)
		if err != nil {
			return nil, err
		}
		source = boundedComplianceDocument(source, 60000)
		docs[art.RecordID] = source
		coverage.SourceRecords++
		if !source.Complete {
			coverage.SourceComplete = false
			coverage.Issues = append(coverage.Issues, source.Issues...)
		}
		var own []complianceArtifact
		for _, v := range *artifacts {
			if v.RecordID == art.RecordID {
				own = append(own, v)
			}
		}
		payload, err := r.assessmentCall(ctx, "requirements", cfg, map[string]any{"activity": a, "source": source, "candidates": own})
		if err != nil {
			return nil, err
		}
		reqs, err := complianceArray[complianceRequirement](payload, "requirements")
		if err != nil {
			return nil, err
		}
		for _, req := range reqs {
			if req.Text == "" || (req.Kind != "quantitative" && req.Kind != "qualitative") || !complianceOneOf(req.Binding, "required", "recommended", "informative", "unknown") {
				return nil, fmt.Errorf("compliance: malformed extracted requirement")
			}
			if req.Source.RecordID != art.RecordID || len(req.ArtifactKeys) == 0 {
				return nil, fmt.Errorf("compliance: requirement cites an unretrieved source")
			}
			for _, key := range req.ArtifactKeys {
				if !seenArtifact[key] {
					return nil, fmt.Errorf("compliance: requirement cites an unretrieved source")
				}
				ownKey := false
				for _, v := range own {
					if v.key() == key {
						ownKey = true
					}
				}
				if !ownKey {
					return nil, fmt.Errorf("compliance: requirement artifact belongs to another source")
				}
			}
			if err = verifyComplianceEvidence(req.Source, docs); err != nil {
				return nil, err
			}
			for _, e := range req.ScopeEvidence {
				if e.RecordID != art.RecordID {
					return nil, fmt.Errorf("compliance: scope evidence belongs to another source")
				}
				if err = verifyComplianceEvidence(e, docs); err != nil {
					return nil, err
				}
			}
			key := fmt.Sprintf("%d/%d/%d/%s", req.Source.RecordID, req.Source.LineStart, req.Source.LineEnd, strings.ToLower(strings.TrimSpace(req.Text)))
			if dedup[key] {
				continue
			}
			dedup[key] = true
			if len(out) >= 32 {
				coverage.SourceComplete = false
				coverage.Issues = append(coverage.Issues, "Requirement budget exhausted.")
				break
			}
			req.ID = fmt.Sprintf("r%d", len(out)+1)
			out = append(out, req)
		}
		deps, err := complianceArray[string](payload, "dependencies")
		if err != nil {
			return nil, err
		}
		if len(deps) > 3 {
			coverage.SourceComplete = false
			coverage.Issues = append(coverage.Issues, "Source dependency budget exceeded.")
			deps = deps[:3]
		}
		for _, dep := range deps {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				return nil, fmt.Errorf("compliance: empty source dependency")
			}
			exclude, err := r.excludeIDs(ctx, coverage.ReviewedRecordID)
			if err != nil {
				return nil, err
			}
			found := false
			for _, typ := range []string{"provision", "metric", "chunk"} {
				results, err := r.searchCandidates(ctx, searchengine.Request{Query: searchengine.Query{Text: dep}, Filters: searchengine.Filters{ArtifactTypes: []string{typ}, ExcludeRecordIDs: exclude}, Options: searchengine.Options{Limit: complianceSearchLimit}})
				if err != nil {
					return nil, err
				}
				coverage.DependencySearches = append(coverage.DependencySearches, complianceRetrievalTrace{Query: dep, ArtifactType: typ, Limit: complianceSearchLimit, PotentiallyTruncated: len(results) >= complianceSearchLimit, Results: results})
				for _, hit := range results {
					candidateDoc, err := r.document(ctx, hit.InputRecordID)
					if err != nil {
						return nil, err
					}
					if !matchesComplianceCitation(dep, candidateDoc) {
						continue
					}
					found = true
					next := complianceArtifact{Type: hit.ArtifactType, ID: hit.ArtifactID, RecordID: hit.InputRecordID}
					if !seenArtifact[next.key()] {
						seenArtifact[next.key()] = true
						*artifacts = append(*artifacts, next)
					}
				}
			}
			if !found {
				coverage.SourceComplete = false
				coverage.Issues = append(coverage.Issues, "Unresolved source dependency: "+dep)
			}
		}
	}
	return out, nil
}

func (r *complianceReviewer) screenDocument(ctx context.Context, a complianceActivity, cfg ReviewerConfig, reqs []complianceRequirement, doc complianceDocument, coverage *complianceCoverage) ([]complianceScreening, error) {
	maxChunks := r.maxEvidenceChunks
	if maxChunks <= 0 {
		maxChunks = 32
	}
	var out []complianceScreening
	wanted := map[string]bool{}
	for _, req := range reqs {
		wanted[req.ID] = true
	}
	for offset := 0; offset < len(doc.Lines); offset += DefaultChunkInputSize {
		if offset/DefaultChunkInputSize >= maxChunks {
			coverage.EvidenceComplete = false
			coverage.Issues = append(coverage.Issues, "Whole-document screening budget exhausted.")
			coverage.UnexaminedRanges = append(coverage.UnexaminedRanges, complianceLineRange{Start: doc.Lines[offset].LineNumber, End: doc.Lines[len(doc.Lines)-1].LineNumber})
			break
		}
		end := min(offset+DefaultChunkInputSize, len(doc.Lines))
		part := doc
		part.Lines = doc.Lines[offset:end]
		part = boundedComplianceDocument(part, 60000)
		if len(part.Lines) != end-offset {
			coverage.EvidenceComplete = false
			coverage.Issues = append(coverage.Issues, "Evidence chunk exceeded character budget.")
			coverage.UnexaminedRanges = append(coverage.UnexaminedRanges, complianceLineRange{Start: doc.Lines[offset+len(part.Lines)].LineNumber, End: doc.Lines[end-1].LineNumber})
		}
		if len(part.Lines) == 0 {
			continue
		}
		payload, err := r.assessmentCall(ctx, "screen", cfg, map[string]any{"activity": a, "requirements": reqs, "document": part})
		if err != nil {
			return nil, err
		}
		screens, err := complianceArray[complianceScreening](payload, "screenings")
		if err != nil {
			return nil, err
		}
		if len(screens) != len(reqs) {
			return nil, fmt.Errorf("compliance: evidence screening omitted requirements")
		}
		seen := map[string]bool{}
		for _, screen := range screens {
			if !wanted[screen.RequirementID] || seen[screen.RequirementID] {
				return nil, fmt.Errorf("compliance: unknown or duplicate screening requirement")
			}
			seen[screen.RequirementID] = true
			for _, e := range screen.Evidence {
				if err = verifyComplianceEvidence(e, map[int64]complianceDocument{doc.RecordID: part}); err != nil {
					return nil, err
				}
			}
		}
		coverage.ScreenedLines += len(part.Lines)
		coverage.ScreenedRanges = append(coverage.ScreenedRanges, complianceLineRange{Start: part.Lines[0].LineNumber, End: part.Lines[len(part.Lines)-1].LineNumber})
		out = append(out, screens...)
	}
	return out, nil
}

func complianceOneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}

func validateComplianceAssessments(results []complianceAssessment, reqs []complianceRequirement, docs map[int64]complianceDocument, coverage complianceCoverage) error {
	if len(results) != len(reqs) {
		return fmt.Errorf("compliance: assessment omitted requirements")
	}
	byID := map[string]complianceRequirement{}
	for _, req := range reqs {
		byID[req.ID] = req
	}
	seen := map[string]bool{}
	for i := range results {
		result := &results[i]
		req, ok := byID[result.RequirementID]
		if !ok || seen[result.RequirementID] || result.Rationale == "" || result.ApplicabilityReason == "" || !complianceOneOf(result.Applicability, "applicable", "not_applicable", "unknown") || !complianceOneOf(result.Status, "supported", "contradicted", "partially_supported", "not_documented", "insufficient_evidence", "not_assessed") {
			return fmt.Errorf("compliance: invalid or duplicate requirement assessment")
		}
		seen[result.RequirementID] = true
		for _, e := range result.Evidence {
			if err := verifyComplianceEvidence(e, docs); err != nil {
				return err
			}
		}
		if req.Binding != "required" || result.Applicability != "applicable" {
			result.Status = "not_assessed"
			result.Suggestion = ""
			continue
		}
		if !coverage.SourceComplete {
			result.Status = "insufficient_evidence"
			result.Suggestion = ""
			result.ResultNote = "Source context is incomplete."
			continue
		}
		if result.Status == "not_documented" && (!coverage.EvidenceComplete || coverage.ScreenedLines != coverage.TotalLines || coverage.TotalLines == 0) {
			result.Status = "insufficient_evidence"
			result.Suggestion = ""
			result.ResultNote = "Whole-document screening is incomplete, so an omission cannot be concluded."
		}
		targetEvidence := false
		for _, e := range result.Evidence {
			if e.RecordID == coverage.ReviewedRecordID {
				targetEvidence = true
			}
		}
		if complianceOneOf(result.Status, "supported", "contradicted", "partially_supported") && !targetEvidence {
			result.Status = "insufficient_evidence"
			result.Suggestion = ""
			result.ResultNote = "No reviewed-document evidence was cited."
		}
		if req.Kind == "quantitative" && complianceOneOf(result.Status, "supported", "contradicted", "partially_supported") {
			if result.Numeric == nil {
				result.Status = "insufficient_evidence"
				result.NumericCheck = "Structured quantities were not supplied."
			} else {
				result.Status, result.NumericCheck = checkComplianceNumeric(*result.Numeric)
			}
			if result.Status == "insufficient_evidence" {
				result.Suggestion = ""
			}
		}
	}
	return nil
}
