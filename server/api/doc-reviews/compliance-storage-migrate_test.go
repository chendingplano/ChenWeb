package docreviews

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const legacyGroupDescription = "Candidate verdicts: 1 compatible, 1 unqualified\n" +
	"compatible [provision 2_prv_1] record 2 (activity a1: seal): s1\n" +
	"unqualified [provision 2_prv_1] record 2 (activity a2: t): s2\n" +
	"  Note: no limit stated\n\n" +
	"Activity a1 [provision, lines 3-3]: seal\n" +
	"Activity a2 [metric, lines 4-4]: t\n\n" +
	"Judged 1 candidate artifact(s) from the knowledge base:\n" +
	"- [provision 2_prv_1] (cosine 0.712) record 2 GB/T 1 Logs: Keep logs.\nfor six months."

// legacyGroupRow is a version-3 grouped finding as the reviewer stored it before the storage
// change; edit tweaks its artifact fields and metadata.
func legacyGroupRow(t *testing.T, edit func(af, meta map[string]any)) legacyComplianceRow {
	t.Helper()
	af := map[string]any{
		"version": 3, "assessment_date": "2026-11-23", "document_level": false,
		"activities": []any{
			map[string]any{"activity_id": "a1", "kind": "provision", "label": "seal", "text": "Seal it.", "line_start": 3, "line_end": 3, "artifact_id": "7_prv_1", "row_id": 11},
			map[string]any{"activity_id": "a2", "kind": "metric", "label": "t", "text": "t = 5", "line_start": 4, "line_end": 4, "artifact_id": "7_mtc_1", "row_id": 12,
				"cited_documents": []any{map[string]any{"name": "GB/T 9"}}},
		},
		"retrieval_trace": []any{
			map[string]any{"activity_id": "a1", "purpose": "activity", "reference": "provision/7_prv_1", "artifact_types": []any{"provision", "metric"},
				"excluded_record_ids": []any{7}, "limit": 30, "potentially_truncated": false, "results": []any{
					map[string]any{"artifact_type": "provision", "artifact_id": "2_prv_1", "record_id": 2, "title": "f2.json", "rank": 1, "score": 0.03, "diag": map[string]any{"cosine": 0.7123}},
					map[string]any{"artifact_type": "provision", "artifact_id": "3_prv_1", "record_id": 3, "title": "f3.json", "rank": 2, "score": 0.02, "diag": map[string]any{"cosine": 0.5}}}},
			map[string]any{"activity_id": "a2", "purpose": "activity", "reference": "metric/7_mtc_1", "artifact_types": []any{"metric", "provision"},
				"excluded_record_ids": []any{7}, "limit": 30, "potentially_truncated": false, "results": []any{
					map[string]any{"artifact_type": "provision", "artifact_id": "2_prv_1", "record_id": 2, "title": "f2.json", "rank": 1, "score": 0.01, "diag": map[string]any{"cosine": 0.65}}}},
		},
		"relevance": []any{
			map[string]any{"activity_id": "a1", "artifact_key": "provision/2_prv_1", "relevant": "yes", "score": 7, "kept": true},
			map[string]any{"activity_id": "a1", "artifact_key": "provision/3_prv_1", "relevant": "no", "score": 1, "kept": false},
			map[string]any{"activity_id": "a2", "artifact_key": "provision/2_prv_1", "relevant": "yes", "score": 5, "kept": true},
		},
		"candidate_verdicts": []any{
			map[string]any{"activity_id": "a1", "artifact_key": "provision/2_prv_1", "verdict": "compatible", "binding": "mandatory", "summary": "s1",
				"candidate_quote": "Keep logs.", "document_quote": "Seal it.", "document_line_start": 3, "document_line_end": 3, "draft_verdict": "compatible"},
			map[string]any{"activity_id": "a2", "artifact_key": "provision/2_prv_1", "verdict": "unqualified", "binding": "none", "summary": "s2",
				"note": "no limit stated", "draft_verdict": "unqualified"},
		},
		"verification": nil,
		"prompts":      map[string]any{"judgement": "prompt-review-compliance-judgement-v2.md"},
		"models":       map[string]any{"primary": "p", "correction": "c", "fallback": "f", "judgement": "p", "relevance": "p"},
		"errors":       nil,
	}
	title := "Compliance assessment: seal (+1 more)"
	meta := map[string]any{"schema_version": 1, "run_id": 116, "model_name": "p", "result_kind": "compliance_assessment",
		"canonical_language": "zh", "canonical_origin": "original", "source_language": "zh", "source_language_confidence": 1,
		"zh": map[string]any{"title": title, "description": legacyGroupDescription, "suggestion": "", "provenance": "canonical"}}
	if edit != nil {
		edit(af, meta)
	}
	meta["artifact_fields"] = af
	b, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	return legacyComplianceRow{ID: 11900, RunID: 116, FindingType: "compliance_assessment", Title: title, Description: legacyGroupDescription,
		Evidence: "Seal it.\nt = 5", Location: "3-4", Metadata: b}
}

func TestBuildLegacyComplianceFindingRebuildsAGroup(t *testing.T) {
	f, translations, err := buildLegacyComplianceFinding(legacyGroupRow(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(translations) != 0 || f.Description != "" || f.LineStart != 3 || f.LineEnd != 4 {
		t.Fatalf("translations %v, description %q, lines %d-%d", translations, f.Description, f.LineStart, f.LineEnd)
	}
	rec := f.Compliance
	if c := rec.counts(); c.Activities != 2 || c.SearchResults != 3 || c.Selected != 3 || c.Relevant != 2 || c.Judged != 2 ||
		c.Verdicts["compatible"] != 1 || c.Verdicts["unqualified"] != 1 {
		t.Fatalf("counts = %+v", c)
	}
	if rec.StageModels["judgement"] != "p" || rec.StageModels["primary"] != "" || rec.Languages.SourceConfidence != 1 {
		t.Errorf("stage models %v, languages %+v", rec.StageModels, rec.Languages)
	}
	a1, a2 := rec.Activities[0], rec.Activities[1]
	if a1.SourceType != "provision" || a1.SourceID != "7_prv_1" || a1.SourceRowID != 11 || a1.SourceText != "Seal it." || len(a1.Searches) != 1 {
		t.Errorf("activity a1 = %+v", a1)
	}
	if _, ok := a2.Details["cited_documents"]; !ok {
		t.Errorf("activity a2 details = %v, want its cited documents", a2.Details)
	}
	judged := a1.Candidates[0]
	// The judged candidate takes record, title, excerpt and judgement order from the list,
	// and its cosine from the first search that returned it.
	if judged.Status != candidateJudged || judged.Title != "GB/T 1 Logs" || judged.Evidence["candidate_excerpt"] != "Keep logs.\nfor six months." ||
		judged.JudgementOrder == nil || *judged.JudgementOrder != 0 || *judged.Cosine != 0.7123 || judged.DisplayOrder != 0 ||
		judged.Evidence["candidate_quote"] != "Keep logs." {
		t.Errorf("judged candidate = %+v", judged)
	}
	if r := a1.Candidates[1]; r.Status != candidateRejected || r.Verdict != "" || r.Title != "f3.json" || r.DisplayOrder != 2 {
		t.Errorf("rejected candidate = %+v", r)
	}
	if u := a2.Candidates[0]; u.Verdict != verdictUnqualified || u.Note != "no limit stated" || u.DisplayOrder != 1 || *u.Cosine != 0.7123 {
		t.Errorf("a2 candidate = %+v", u)
	}
}

func TestBuildLegacyComplianceFindingValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(af, meta map[string]any)
		row  func(*legacyComplianceRow)
		want string
	}{
		{name: "unresolved label", want: "names activity a3", edit: func(af, _ map[string]any) {
			af["relevance"] = append(af["relevance"].([]any), map[string]any{"activity_id": "a3", "artifact_key": "provision/9_prv_1", "kept": false})
		}},
		{name: "verdict without kept relevance", want: "has no kept relevance decision", edit: func(af, _ map[string]any) {
			af["relevance"].([]any)[2].(map[string]any)["kept"] = false
		}},
		{name: "verdict rows differ", want: "verdict rows do not match", row: func(r *legacyComplianceRow) {
			r.Description = strings.Replace(r.Description, ": s1\n", ": changed\n", 1)
		}},
		{name: "cosine differs", want: "cosine differs", row: func(r *legacyComplianceRow) {
			r.Description = strings.Replace(r.Description, "(cosine 0.712)", "(cosine 0.650)", 1)
		}},
		{name: "judged list missing", want: "no list of 1 judged candidates", row: func(r *legacyComplianceRow) {
			r.Description = r.Description[:strings.Index(r.Description, "\n\nJudged")]
		}},
		{name: "canonical copy differs", want: "zh copy differs", edit: func(_, meta map[string]any) {
			meta["zh"].(map[string]any)["title"] = "something else"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := legacyGroupRow(t, tc.edit)
			if tc.row != nil {
				tc.row(&row)
			}
			if _, _, err := buildLegacyComplianceFinding(row); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// A copy in another language is a real translation and becomes a translation row; a finding
// without recorded models still migrates, with no stage models (nothing is filled in).
func TestBuildLegacyComplianceFindingTranslationsAndMissingHistory(t *testing.T) {
	row := legacyGroupRow(t, func(af, meta map[string]any) {
		meta["en"] = map[string]any{"title": "T", "description": "D", "provenance": "llm_translation"}
		delete(af, "models")
		delete(af, "prompts")
	})
	f, translations, err := buildLegacyComplianceFinding(row)
	if err != nil {
		t.Fatal(err)
	}
	if tr, ok := translations["en"]; !ok || tr.Title != "T" || tr.Provenance != "llm_translation" || len(translations) != 1 {
		t.Errorf("translations = %+v", translations)
	}
	if len(f.Compliance.StageModels) != 0 {
		t.Errorf("stage models = %v, want none", f.Compliance.StageModels)
	}
}

func TestBuildLegacyComplianceFindingShapes(t *testing.T) {
	old := legacyComplianceRow{ID: 1, FindingType: "compliance_grounding_candidates", Description: "Retrieved 25 candidate artifact(s)",
		Location: "18-18", Metadata: []byte(`{"schema_version":1,"en":{"title":"x"}}`)}
	if _, _, err := buildLegacyComplianceFinding(old); !errors.Is(err, errUnsupportedShape) {
		t.Errorf("old shape: %v", err)
	}
	notice := legacyComplianceRow{ID: 2, FindingType: "compliance_unchecked", Title: "Activities not checked because of limits",
		Description: "3 metric(s) were not checked.", Location: "1-200",
		Metadata: []byte(`{"schema_version":1,"canonical_language":"en","en":{"title":"Activities not checked because of limits","description":"3 metric(s) were not checked."},` +
			`"artifact_fields":{"unchecked_metrics":3,"unchecked_provisions":0,"without_lines":0,"max_metrics":100,"max_provisions":100}}`)}
	f, _, err := buildLegacyComplianceFinding(notice)
	if err != nil || len(f.Compliance.Activities) != 0 || f.Compliance.Extra["unchecked_metrics"] != 3 || f.Description != notice.Description {
		t.Fatalf("notice = %+v, err %v", f.Compliance, err)
	}
	detect := legacyComplianceRow{ID: 3, FindingType: "compliance_incomplete", Title: "Compliance review incomplete: text activities of lines 1-200",
		Description: "The text activities of lines 1-200 could not be found.", ErrorMsg: "timeout", Location: "1-200",
		Metadata: []byte(`{"schema_version":1,"result_kind":"compliance_activity"}`)}
	if f, _, err := buildLegacyComplianceFinding(detect); err != nil || f.ErrorMsg != "timeout" || len(f.Compliance.Activities) != 0 {
		t.Fatalf("detect failure = %+v, err %v", f, err)
	}
}

// A failed judgement batch leaves kept candidates without verdicts; they are recorded as
// judgement failures, not as pending.
func TestBuildLegacyComplianceFindingJudgementFailure(t *testing.T) {
	desc := "The review of this activity stopped because of an error; see the error message.\n\n" +
		"Activity a1 [provision, lines 3-3]: seal\nActivity a2 [metric, lines 4-4]: t"
	row := legacyGroupRow(t, func(af, meta map[string]any) {
		af["candidate_verdicts"] = []any{}
		af["errors"] = []any{"judgement of 2 candidate(s) from activity a1 on: timeout"}
		meta["zh"].(map[string]any)["description"] = desc
		meta["zh"].(map[string]any)["title"] = "Compliance review incomplete: seal (+1 more)"
	})
	row.FindingType, row.Title, row.Description = "compliance_incomplete", "Compliance review incomplete: seal (+1 more)", desc
	f, _, err := buildLegacyComplianceFinding(row)
	if err != nil {
		t.Fatal(err)
	}
	c := f.Compliance.Activities[0].Candidates
	if c[0].Status != candidateJudgementFailed || c[1].Status != candidateRejected || f.ErrorMsg == "" ||
		f.Description != "The review of this activity stopped because of an error; see the error message." {
		t.Fatalf("candidates %+v, error %q, description %q", c, f.ErrorMsg, f.Description)
	}
}

func TestStripNeedsARecoveryDirectory(t *testing.T) {
	if _, n, err := StripMigratedComplianceFindings(context.Background(), nil, nil, " "); err == nil || n != 0 {
		t.Fatalf("n %d, err %v", n, err)
	}
}
