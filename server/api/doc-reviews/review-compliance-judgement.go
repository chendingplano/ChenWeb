package docreviews

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// complianceJudgementMaxPairs caps the activity–candidate pairs of one judgement or verification
// call, so a large activity group (a table can give dozens of metrics on overlapping lines) does not
// overrun the model's output limit. A larger unit is judged in several calls.
const complianceJudgementMaxPairs = 20

// complianceVerdict is one activity–candidate verdict, from the judgement call and then the numeric
// comparison and the verifier.
type complianceVerdict struct {
	ActivityID        string             `json:"activity_id"`
	ArtifactKey       string             `json:"artifact_key"`
	Verdict           string             `json:"verdict"`
	Binding           string             `json:"binding"`
	Summary           string             `json:"summary"`
	CandidateQuote    string             `json:"candidate_quote"`
	DocumentQuote     string             `json:"document_quote"`
	DocumentLineStart int                `json:"document_line_start"`
	DocumentLineEnd   int                `json:"document_line_end"`
	Numeric           *complianceNumeric `json:"numeric,omitempty"`
	NumericResult     string             `json:"numeric_result,omitempty"`
	DraftVerdict      string             `json:"draft_verdict,omitempty"`
	Verification      string             `json:"verification,omitempty"`
	// Note says why the verdict became undetermined (numbers or verifier).
	Note string `json:"note,omitempty"`
}

type complianceCheck struct {
	ActivityID  string `json:"activity_id"`
	ArtifactKey string `json:"artifact_key"`
	Valid       bool   `json:"valid"`
	Reason      string `json:"reason"`
}

// complianceUnitState collects what reviewing one unit produced, for its finding.
type complianceUnitState struct {
	unit      *complianceUnit
	pool      map[string]*complianceCandidate
	kept      map[string][]string // activity ID -> kept candidate keys
	traces    []complianceRetrievalTrace
	relevance []complianceRelevanceDecision
	verdicts  []complianceVerdict
	checks    []complianceCheck
	models    map[string]string
	errs      []string
	// document, evidence and lines are what the judgement and verification see of the reviewed
	// document: its doc_context line and the unit's evidence window (see complianceEvidenceWindow).
	document string
	evidence complianceEvidence
	lines    []complianceInputLine
}

// complianceUnitRequest is the input of a judgement or verification call. Its fields run from the
// most shared to the least: the review (every call of the review), the evidence (every call of the
// unit), then this call's activities, candidates and draft verdicts. With the prompt as the system
// message (TaskFirst), calls that share a review or a unit share a cacheable prefix.
type complianceUnitRequest struct {
	Review     complianceReviewContext `json:"review"`
	Evidence   complianceEvidence      `json:"evidence"`
	Activities []map[string]any        `json:"activities"`
	Candidates []*complianceCandidate  `json:"candidates"`
	Verdicts   []complianceVerdict     `json:"verdicts,omitempty"`
}

type complianceReviewContext struct {
	Document       string `json:"document"`
	AssessmentDate string `json:"assessment_date"`
	OutputLanguage string `json:"output_language"`
}

// complianceEvidence is the part of the chunk a unit is judged on, in the chunk's compact form: a
// header row, then one row per line.
type complianceEvidence struct {
	Lines [][]any `json:"lines"`
}

// complianceEvidenceWindow cuts lines start..end out of the chunk input. It returns the document's
// doc_context line, the rows for the model and the lines for the quote checks. The window cannot
// reach past the chunk, whose input is all the reviewer holds. Rows are decoded so they are written
// again without the HTML escaping of the chunk envelope.
func complianceEvidenceWindow(inputJSON string, start, end int) (string, complianceEvidence, []complianceInputLine) {
	var env struct {
		DocContext string  `json:"doc_context"`
		Lines      [][]any `json:"lines"`
	}
	if err := json.Unmarshal([]byte(inputJSON), &env); err != nil {
		// A chunk without a doc_context line arrives as the bare table.
		env.DocContext = ""
		if json.Unmarshal([]byte(inputJSON), &env.Lines) != nil {
			return "", complianceEvidence{}, nil
		}
	}
	if len(env.Lines) < 2 {
		return env.DocContext, complianceEvidence{}, nil
	}
	numCol, contentCol := -1, -1
	for i, h := range env.Lines[0] {
		switch h {
		case "line_number":
			numCol = i
		case "content":
			contentCol = i
		}
	}
	if numCol < 0 || contentCol < 0 {
		return env.DocContext, complianceEvidence{}, nil
	}
	ev := complianceEvidence{Lines: [][]any{env.Lines[0]}}
	var lines []complianceInputLine
	for _, row := range env.Lines[1:] {
		if len(row) <= numCol || len(row) <= contentCol {
			continue
		}
		n, _ := row[numCol].(float64)
		if int(n) < start || int(n) > end {
			continue
		}
		c, _ := row[contentCol].(string)
		ev.Lines = append(ev.Lines, row)
		lines = append(lines, complianceInputLine{LineNumber: int(n), Content: c})
	}
	return env.DocContext, ev, lines
}

// complianceRequestJSON writes a call's input without HTML escaping, so "<" and "&" in the document
// and the candidates reach the model as written, not as < and &.
func complianceRequestJSON(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

func (s *complianceUnitState) fail(format string, args ...any) {
	s.errs = append(s.errs, fmt.Sprintf(format, args...))
}

// reviewUnit reviews one text activity or activity group. It never returns an error: a failure is
// recorded in the finding's error message and the reviewer moves on.
func (r *complianceReviewer) reviewUnit(ctx context.Context, recordID int64, chunkIndex int, cfg ReviewerConfig, input chunkInput, u *complianceUnit) ReviewFinding {
	s := &complianceUnitState{unit: u, pool: map[string]*complianceCandidate{}, kept: map[string][]string{}, models: map[string]string{}}
	exclude, err := r.excludeIDs(ctx, recordID)
	if err != nil {
		s.fail("excluded records: %v", err)
		return r.unitFinding(s, cfg)
	}
	found := map[string][]string{}
	for _, m := range u.Members {
		res, err := r.searchMember(ctx, exclude, m, s.pool)
		s.traces = append(s.traces, res.Traces...)
		if err != nil {
			s.fail("%s: %v", m.Label, err)
			continue
		}
		found[m.ID] = res.Found
	}
	if err := r.loadCandidateTexts(ctx, s.pool); err != nil {
		s.fail("%v", err)
		return r.unitFinding(s, cfg)
	}
	for _, m := range u.Members {
		if len(found[m.ID]) == 0 {
			continue
		}
		kept, decisions, model, err := r.checkRelevance(ctx, recordID, chunkIndex, cfg, m, found[m.ID], s.pool)
		if err != nil {
			s.fail("%s: %v", m.Label, err)
			continue
		}
		s.models["relevance"] = model
		s.relevance = append(s.relevance, decisions...)
		if len(kept) > 0 {
			s.kept[m.ID] = kept
		}
	}
	if len(s.kept) == 0 {
		return r.unitFinding(s, cfg)
	}
	n := r.settings.EvidenceLines
	s.document, s.evidence, s.lines = complianceEvidenceWindow(input.inputJSON, u.LineStart-n, u.LineEnd+n)
	var pairs []string
	for _, m := range u.Members {
		for _, k := range s.kept[m.ID] {
			pairs = append(pairs, m.ID+"\x00"+k)
		}
	}
	for _, batch := range complianceBatches(pairs, complianceJudgementMaxPairs) {
		verdicts, model, err := r.judge(ctx, recordID, chunkIndex, cfg, s, batch)
		if err != nil {
			id, _, _ := strings.Cut(batch[0], "\x00")
			s.fail("judgement of %d candidate(s) from activity %s on: %v", len(batch), id, err)
			continue
		}
		s.models["judgement"] = model
		for i := range verdicts {
			verdicts[i].DraftVerdict = verdicts[i].Verdict
			applyComplianceNumeric(&verdicts[i])
		}
		s.verdicts = append(s.verdicts, verdicts...)
	}
	r.verify(ctx, recordID, chunkIndex, cfg, s)
	return r.unitFinding(s, cfg)
}

// complianceBatches splits pairs into consecutive batches of at most n.
func complianceBatches(pairs []string, n int) [][]string {
	var out [][]string
	for len(pairs) > n {
		out = append(out, pairs[:n])
		pairs = pairs[n:]
	}
	if len(pairs) > 0 {
		out = append(out, pairs)
	}
	return out
}

func complianceKeySet(pairs []string) map[string]bool {
	keys := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		keys[p] = true
	}
	return keys
}

// unitRequest is the input of a judgement or verification call: the review, the unit's evidence,
// the unit's activities, each with the candidate keys of the given pairs (all kept candidates when
// keys is nil), and those candidates.
func (r *complianceReviewer) unitRequest(s *complianceUnitState, keys map[string]bool) complianceUnitRequest {
	acts := make([]map[string]any, 0, len(s.unit.Members))
	for _, m := range s.unit.Members {
		var candKeys []string
		for _, k := range s.kept[m.ID] {
			if keys == nil || keys[m.ID+"\x00"+k] {
				candKeys = append(candKeys, k)
			}
		}
		if len(candKeys) == 0 {
			continue
		}
		v := complianceMemberView(m)
		v["candidate_keys"] = candKeys
		acts = append(acts, v)
	}
	var cands []*complianceCandidate
	seen := map[string]bool{}
	for _, a := range acts {
		for _, k := range a["candidate_keys"].([]string) {
			if !seen[k] {
				seen[k] = true
				cands = append(cands, s.pool[k])
			}
		}
	}
	return complianceUnitRequest{
		Review:   complianceReviewContext{Document: s.document, AssessmentDate: r.assessmentDate, OutputLanguage: r.language},
		Evidence: s.evidence, Activities: acts, Candidates: cands,
	}
}

func (r *complianceReviewer) judge(ctx context.Context, recordID int64, chunkIndex int, cfg ReviewerConfig, s *complianceUnitState, pairs []string) ([]complianceVerdict, string, error) {
	input, err := complianceRequestJSON(r.unitRequest(s, complianceKeySet(pairs)))
	if err != nil {
		return nil, "", err
	}
	var verdicts []complianceVerdict
	p := r.prompts["judgement"]
	_, model, err := r.callWithTries(ctx, cfg, complianceCall{
		Stage: "judgement", PromptRef: p.Ref, PromptText: p.Text, Input: input, Schema: complianceJudgementSchema(),
		RecordID: recordID, ChunkIndex: &chunkIndex, TaskFirst: true,
		Check: func(parsed map[string]any) error {
			var cerr error
			verdicts, cerr = checkComplianceJudgement(parsed, s, pairs, s.lines)
			return cerr
		},
	})
	if err != nil {
		return nil, "", err
	}
	return verdicts, model, nil
}

// checkComplianceJudgement decodes the verdicts and reports every problem at once, naming the
// activity, the candidate and the field.
func checkComplianceJudgement(parsed map[string]any, s *complianceUnitState, order []string, lines []complianceInputLine) ([]complianceVerdict, error) {
	verdicts, err := complianceArray[complianceVerdict](parsed, "verdicts")
	if err != nil {
		return nil, fmt.Errorf("%v; return {\"verdicts\":[...]} with one entry per activity and candidate key", err)
	}
	expected := complianceKeySet(order)
	got := map[string]bool{}
	var problems []string
	for i, v := range verdicts {
		pair := v.ActivityID + "\x00" + v.ArtifactKey
		where := fmt.Sprintf("Verdict %d (activity %s, candidate %s)", i+1, v.ActivityID, v.ArtifactKey)
		if !expected[pair] {
			problems = append(problems, fmt.Sprintf("%s: this pair was not asked for; judge only the candidate_keys listed under each activity", where))
			continue
		}
		if got[pair] {
			problems = append(problems, fmt.Sprintf("%s: repeated; return exactly one verdict per pair", where))
			continue
		}
		got[pair] = true
		problems = append(problems, checkComplianceVerdict(where, v, s.pool[v.ArtifactKey], lines)...)
		if verdicts[i].Binding == "" {
			verdicts[i].Binding = "none"
		}
	}
	for _, pair := range order {
		if !got[pair] {
			id, key, _ := strings.Cut(pair, "\x00")
			problems = append(problems, fmt.Sprintf("Activity %s, candidate %s: no verdict; return one", id, key))
		}
	}
	if len(problems) > 0 {
		return nil, errors.New(strings.Join(problems, "\n"))
	}
	return verdicts, nil
}

func checkComplianceVerdict(where string, v complianceVerdict, c *complianceCandidate, lines []complianceInputLine) []string {
	var problems []string
	if !complianceOneOf(v.Verdict, verdictCompliant, verdictCompatible, verdictNotCompliant, verdictUndetermined, verdictUnqualified) {
		problems = append(problems, fmt.Sprintf("%s: verdict %q is not one of compliant, compatible, not_compliant, undetermined, unqualified", where, v.Verdict))
	}
	if !complianceOneOf(v.Binding, "mandatory", "recommendation", "none", "") {
		problems = append(problems, fmt.Sprintf("%s: binding %q is not one of mandatory, recommendation, none", where, v.Binding))
	}
	if strings.TrimSpace(v.Summary) == "" {
		problems = append(problems, fmt.Sprintf("%s: summary is empty; write what the candidate requires, whether it applies and what the document states", where))
	}
	strong := v.Verdict == verdictCompliant || v.Verdict == verdictNotCompliant
	if strong && v.Binding != "mandatory" {
		problems = append(problems, fmt.Sprintf("%s: %s needs binding mandatory; a recommendation or a candidate with no requirement is compatible, undetermined or unqualified", where, v.Verdict))
	}
	if strong && strings.TrimSpace(v.CandidateQuote) == "" {
		problems = append(problems, fmt.Sprintf("%s: %s needs candidate_quote, the exact candidate text that states the requirement", where, v.Verdict))
	}
	if strong && strings.TrimSpace(v.DocumentQuote) == "" {
		problems = append(problems, fmt.Sprintf("%s: %s needs document_quote, the exact evidence text that shows what the document states, with its lines", where, v.Verdict))
	}
	// compatible says the candidate states a requirement that may apply, and a binding says what that
	// requirement is: both need the candidate text that states it. A bare name states none.
	if v.Verdict == verdictCompatible && !complianceOneOf(v.Binding, "mandatory", "recommendation") {
		problems = append(problems, fmt.Sprintf("%s: compatible needs binding mandatory or recommendation, because it says the candidate's requirement may apply; a candidate that states no requirement is unqualified with binding none", where))
	}
	if q := strings.TrimSpace(v.CandidateQuote); complianceOneOf(v.Binding, "mandatory", "recommendation") {
		switch {
		case q == "" && !strong: // a strong verdict without a quote is reported above
			problems = append(problems, fmt.Sprintf("%s: binding %s needs candidate_quote, the exact candidate text that states the duty or limit; if the candidate states none, set binding none and verdict unqualified", where, v.Binding))
		case q != "" && c != nil && c.Name != "" && complianceLooseText(q) == complianceLooseText(c.Name):
			problems = append(problems, fmt.Sprintf("%s: candidate_quote %q is only the candidate's name, which states no requirement; quote the text that states the duty or limit, or set binding none and verdict unqualified", where, complianceCap(q, 120)))
		}
	}
	if q := strings.TrimSpace(v.CandidateQuote); q != "" && c != nil && !strings.Contains(complianceLooseText(c.Text), complianceLooseText(q)) {
		problems = append(problems, fmt.Sprintf("%s: candidate_quote %q does not occur in the candidate's text; copy it exactly from the candidate", where, complianceCap(q, 120)))
	}
	if q := strings.TrimSpace(v.DocumentQuote); q != "" {
		if err := complianceQuoteInLines(lines, v.DocumentLineStart, v.DocumentLineEnd, q, true); err != nil {
			problems = append(problems, fmt.Sprintf("%s: document_quote %v; copy it exactly from the evidence lines and give their numbers", where, err))
		}
	}
	return problems
}

func complianceJudgementSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []any{"verdicts"},
		"properties": map[string]any{
			"verdicts": map[string]any{"type": "array", "items": map[string]any{
				"type":     "object",
				"required": []any{"activity_id", "artifact_key", "verdict", "summary"},
				"properties": map[string]any{
					"activity_id":  map[string]any{"type": "string"},
					"artifact_key": map[string]any{"type": "string"},
					"verdict":      map[string]any{"type": "string", "enum": []any{verdictCompliant, verdictCompatible, verdictNotCompliant, verdictUndetermined, verdictUnqualified}},
					"binding":      map[string]any{"type": "string", "enum": []any{"mandatory", "recommendation", "none"}},
					"summary":      map[string]any{"type": "string"},
				},
			}},
		},
	}
}

// applyComplianceNumeric lets the quantity-comparison module decide a verdict that carries quantities.
func applyComplianceNumeric(v *complianceVerdict) {
	if v.Numeric == nil || v.Binding != "mandatory" || strings.TrimSpace(v.CandidateQuote) == "" || strings.TrimSpace(v.DocumentQuote) == "" ||
		!complianceOneOf(v.Verdict, verdictCompliant, verdictCompatible, verdictNotCompliant) {
		return
	}
	status, reason := checkComplianceNumeric(*v.Numeric)
	v.NumericResult = status + ": " + reason
	switch status {
	case "supported":
		v.Verdict = verdictCompliant
	case "contradicted":
		v.Verdict = verdictNotCompliant
	default:
		v.Verdict = verdictUndetermined
		v.Note = "The quantities could not be compared: " + reason
	}
}

// verify checks every compliant and not compliant verdict. A rejected verdict, or every strong
// verdict when the verification call fails, becomes undetermined with the reason as note.
func (r *complianceReviewer) verify(ctx context.Context, recordID int64, chunkIndex int, cfg ReviewerConfig, s *complianceUnitState) {
	var strong []complianceVerdict
	for _, v := range s.verdicts {
		if v.Verdict == verdictCompliant || v.Verdict == verdictNotCompliant {
			strong = append(strong, v)
		}
	}
	for start := 0; start < len(strong); start += complianceJudgementMaxPairs {
		r.verifyBatch(ctx, recordID, chunkIndex, cfg, s, strong[start:min(start+complianceJudgementMaxPairs, len(strong))])
	}
}

func (r *complianceReviewer) verifyBatch(ctx context.Context, recordID int64, chunkIndex int, cfg ReviewerConfig, s *complianceUnitState, strong []complianceVerdict) {
	keys := map[string]bool{}
	for _, v := range strong {
		keys[v.ActivityID+"\x00"+v.ArtifactKey] = true
	}
	in := r.unitRequest(s, keys)
	in.Verdicts = strong
	input, err := complianceRequestJSON(in)
	if err == nil {
		var checks []complianceCheck
		p := r.prompts["verify"]
		var model string
		_, model, err = r.callWithTries(ctx, cfg, complianceCall{
			Stage: "verify", PromptRef: p.Ref, PromptText: p.Text, Input: input, Schema: complianceVerifierSchema(),
			RecordID: recordID, ChunkIndex: &chunkIndex, TaskFirst: true,
			Check: func(parsed map[string]any) error {
				var cerr error
				checks, cerr = checkComplianceChecks(parsed, keys)
				return cerr
			},
		})
		if err == nil {
			s.models["verify"] = model
			s.checks = append(s.checks, checks...)
			byPair := map[string]complianceCheck{}
			for _, c := range checks {
				byPair[c.ActivityID+"\x00"+c.ArtifactKey] = c
			}
			for i := range s.verdicts {
				c, ok := byPair[s.verdicts[i].ActivityID+"\x00"+s.verdicts[i].ArtifactKey]
				if !ok {
					continue
				}
				s.verdicts[i].Verification = c.Reason
				if !c.Valid {
					s.verdicts[i].Verdict = verdictUndetermined
					s.verdicts[i].Note = "Verification rejected the verdict: " + c.Reason
				}
			}
			return
		}
	}
	s.fail("verification: %v", err)
	for i := range s.verdicts {
		if keys[s.verdicts[i].ActivityID+"\x00"+s.verdicts[i].ArtifactKey] {
			s.verdicts[i].Verdict = verdictUndetermined
			s.verdicts[i].Note = "The verdict could not be verified."
		}
	}
}

func checkComplianceChecks(parsed map[string]any, keys map[string]bool) ([]complianceCheck, error) {
	checks, err := complianceArray[complianceCheck](parsed, "checks")
	if err != nil {
		return nil, fmt.Errorf("%v; return {\"checks\":[...]} with one entry per draft verdict", err)
	}
	got := map[string]bool{}
	var problems []string
	for i, c := range checks {
		pair := c.ActivityID + "\x00" + c.ArtifactKey
		where := fmt.Sprintf("Check %d (activity %s, candidate %s)", i+1, c.ActivityID, c.ArtifactKey)
		switch {
		case !keys[pair]:
			problems = append(problems, where+": no such draft verdict; check only the supplied verdicts")
		case got[pair]:
			problems = append(problems, where+": repeated; return exactly one check per verdict")
		case strings.TrimSpace(c.Reason) == "":
			problems = append(problems, where+": reason is empty; say why the verdict holds or what is wrong")
		}
		got[pair] = true
	}
	var missing []string
	for pair := range keys {
		if !got[pair] {
			id, key, _ := strings.Cut(pair, "\x00")
			missing = append(missing, fmt.Sprintf("Activity %s, candidate %s: no check; return one", id, key))
		}
	}
	sort.Strings(missing)
	problems = append(problems, missing...)
	if len(problems) > 0 {
		return nil, errors.New(strings.Join(problems, "\n"))
	}
	return checks, nil
}

func complianceVerifierSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []any{"checks"},
		"properties": map[string]any{
			"checks": map[string]any{"type": "array", "items": map[string]any{
				"type":     "object",
				"required": []any{"activity_id", "artifact_key", "valid", "reason"},
				"properties": map[string]any{
					"activity_id":  map[string]any{"type": "string"},
					"artifact_key": map[string]any{"type": "string"},
					"valid":        map[string]any{"type": "boolean"},
					"reason":       map[string]any{"type": "string"},
				},
			}},
		},
	}
}

// unitFinding writes one finding for a unit: the verdicts when there are any, "no grounding" when
// nothing was left to judge, and "incomplete" when an error stopped the review before any verdict.
func (r *complianceReviewer) unitFinding(s *complianceUnitState, cfg ReviewerConfig) ReviewFinding {
	u := s.unit
	label := u.label()
	f := ReviewFinding{
		Pass: "P5", Aspect: "compliance", Severity: "info", ResultKind: "compliance_activity",
		Location: fmt.Sprintf("%d-%d", u.LineStart, u.LineEnd), ModelName: cfg.ModelName,
		Evidence: complianceUnitEvidence(u), ErrorMsg: strings.Join(s.errs, "\n"),
	}
	if len(u.Members) > 0 && u.Members[0].Kind != "text" {
		f.ArtifactID = u.Members[0].ArtifactID
	}
	seenDoc := map[string]bool{}
	for _, m := range u.Members {
		for _, d := range m.CitedDocs {
			if !seenDoc[d.Name] {
				seenDoc[d.Name] = true
				f.CitedDocuments = append(f.CitedDocuments, d.Name)
			}
		}
	}

	var b strings.Builder
	switch {
	case len(s.verdicts) > 0:
		f.FindingType, f.ResultKind = "compliance_assessment", "compliance_assessment"
		f.Language = r.language
		f.Title = "Compliance assessment: " + label
		related, text := complianceVerdictRows(s.verdicts, s.pool, u)
		f.RelatedArtifacts = related
		b.WriteString(text)
		for _, v := range s.verdicts {
			if v.Verdict == verdictNotCompliant {
				f.Severity = "medium"
			}
		}
	case len(s.errs) > 0:
		f.FindingType = "compliance_incomplete"
		f.Title = "Compliance review incomplete: " + label
		b.WriteString("The review of this activity stopped because of an error; see the error message.\n\n")
	default:
		f.FindingType = "compliance_no_grounding"
		f.Title = "Activity with no grounding candidates found: " + label
		b.WriteString("No candidate was left after the knowledge-base search and the relevance check. " +
			"This does not mean the activity is unregulated.\n\n")
	}
	for _, m := range u.Members {
		fmt.Fprintf(&b, "Activity %s [%s, lines %d-%d]: %s\n", m.ID, m.Kind, m.LineStart, m.LineEnd, m.Label)
	}
	// The plain candidate list, in the form compliance-score reads: only judged candidates.
	judged := complianceJudgedKeys(s.verdicts)
	if len(judged) > 0 {
		fmt.Fprintf(&b, "\nJudged %d candidate artifact(s) from the knowledge base:\n", len(judged))
		var refs []complianceArtifactRef
		for _, key := range judged {
			c := s.pool[key]
			if c == nil {
				continue
			}
			cos := ""
			if c.Cosine != nil {
				cos = fmt.Sprintf(" (cosine %.3f)", *c.Cosine)
			}
			fmt.Fprintf(&b, "- [%s %s]%s record %d %s: %s\n", c.Type, c.ID, cos, c.RecordID, c.Title, complianceCap(c.Text, 300))
			refs = append(refs, complianceArtifactRef{Type: c.Type, ID: c.ID, RecordID: c.RecordID})
		}
		if len(refs) > 0 {
			f.RelatedArtifactID, f.RelatedRecordID = refs[0].ID, refs[0].RecordID
			f.RelatedArtifactFields, _ = json.Marshal(refs)
		}
	}
	f.Description = b.String()

	prompts := map[string]string{"detect": cfg.PromptRef}
	for stage, p := range r.prompts {
		prompts[stage] = p.Ref
	}
	models := map[string]string{"primary": cfg.ModelName, "correction": r.correction.model, "fallback": r.fallback.model}
	for stage, m := range s.models {
		models[stage] = m
	}
	f.ArtifactFields, _ = json.Marshal(map[string]any{
		"version": 3, "assessment_date": r.assessmentDate, "document_level": u.documentLevel(),
		"activities": u.Members, "retrieval_trace": s.traces, "relevance": s.relevance,
		"candidate_verdicts": orderComplianceVerdicts(s.verdicts), "verification": s.checks,
		"prompts": prompts, "models": models, "errors": s.errs,
	})
	return f
}

// complianceArtifactRef names a judged candidate in related_artifact_fields.
type complianceArtifactRef struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	RecordID int64  `json:"record_id"`
}

func complianceJudgedKeys(verdicts []complianceVerdict) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range verdicts {
		if !seen[v.ArtifactKey] {
			seen[v.ArtifactKey] = true
			out = append(out, v.ArtifactKey)
		}
	}
	return out
}

func complianceUnitEvidence(u *complianceUnit) string {
	var parts []string
	for _, m := range u.Members {
		parts = append(parts, complianceCap(m.Text, 400))
	}
	return strings.Join(parts, "\n")
}

// complianceUncheckedFinding reports the activities the limits, or missing lines, left unchecked.
func complianceUncheckedFinding(p *compliancePlan, s complianceSettings, input chunkInput, model string) (ReviewFinding, bool) {
	if p.UncheckedMetrics == 0 && p.UncheckedProvisions == 0 && p.WithoutLines == 0 {
		return ReviewFinding{}, false
	}
	var b strings.Builder
	if p.UncheckedMetrics > 0 || p.UncheckedProvisions > 0 {
		fmt.Fprintf(&b, "%d metric(s) and %d provision(s) were not checked: at most %d metrics (max_metrics) and %d provisions (max_provisions) are checked, the first in the order they appear.\n",
			p.UncheckedMetrics, p.UncheckedProvisions, s.MaxMetrics, s.MaxProvisions)
	}
	if p.WithoutLines > 0 {
		fmt.Fprintf(&b, "%d metric(s) or provision(s) have no source lines and were not checked.\n", p.WithoutLines)
	}
	artifactFields, _ := json.Marshal(map[string]any{"unchecked_metrics": p.UncheckedMetrics, "unchecked_provisions": p.UncheckedProvisions,
		"without_lines": p.WithoutLines, "max_metrics": s.MaxMetrics, "max_provisions": s.MaxProvisions})
	return ReviewFinding{Pass: "P5", Aspect: "compliance", Severity: "info", FindingType: "compliance_unchecked", ResultKind: "compliance_unchecked",
		Title: "Activities not checked because of limits", Description: b.String(), ModelName: model,
		Location: fmt.Sprintf("%d-%d", input.startLine, input.endLine), ArtifactFields: artifactFields}, true
}

// complianceDetectFailureFinding reports that the text activities of a chunk could not be found.
func complianceDetectFailureFinding(input chunkInput, err error, model string) ReviewFinding {
	label := fmt.Sprintf("text activities of lines %d-%d", input.startLine, input.endLine)
	return ReviewFinding{Pass: "P5", Aspect: "compliance", Severity: "info", FindingType: "compliance_incomplete", ResultKind: "compliance_activity",
		Title: "Compliance review incomplete: " + label, ModelName: model, ErrorMsg: err.Error(),
		Description: "The " + label + " could not be found because of an error; see the error message. Their metrics and provisions were still reviewed.\n",
		Location:    fmt.Sprintf("%d-%d", input.startLine, input.endLine)}
}
