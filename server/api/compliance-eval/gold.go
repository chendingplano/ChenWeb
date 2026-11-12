package complianceeval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	docreviews "github.com/chendingplano/deepdoc/server/api/doc-reviews"
	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

// Prompt files (in ChenWeb/prompts). The rules file is frozen per version and prepended to every
// task prompt; its SHA-256 is stored with the run.
const (
	RulesFile              = "prompt-compliance-gold-rules-v1.md"
	ActivitiesPromptFile   = "prompt-compliance-gold-activities-v1.md"
	JudgePromptFile        = "prompt-compliance-gold-judge-v1.md"
	AdjActivityPromptFile  = "prompt-compliance-gold-adjudicate-activity-v1.md"
	AdjJudgementPromptFile = "prompt-compliance-gold-adjudicate-judgement-v1.md"

	judgeBatch       = 10
	candidateTextMax = 1200
	activityTextMax  = 600
	overlapThreshold = 0.5
	labelGoverns     = "governs"
	labelTopical     = "topical"
	labelUnrelated   = "unrelated"
	labelAmbiguous   = "ambiguous"
	agreeBoth        = "both"
	agreeAdjudicated = "adjudicated_kept"
)

// Prompts are the composed (rules + task) prompt texts.
type Prompts struct {
	Activities, Judge, AdjActivity, AdjJudgement string
	RulesSHA256                                  string
}

// LoadPrompts reads the rules and the four task prompts through load (docprocessing.LoadPromptByRef
// in production) and composes them.
func LoadPrompts(load func(ref string) (string, error)) (Prompts, error) {
	rules, err := load(RulesFile)
	if err != nil {
		return Prompts{}, fmt.Errorf("load %s: %w", RulesFile, err)
	}
	sum := sha256.Sum256([]byte(rules))
	p := Prompts{RulesSHA256: hex.EncodeToString(sum[:])}
	for ref, dst := range map[string]*string{
		ActivitiesPromptFile: &p.Activities, JudgePromptFile: &p.Judge,
		AdjActivityPromptFile: &p.AdjActivity, AdjJudgementPromptFile: &p.AdjJudgement,
	} {
		task, err := load(ref)
		if err != nil {
			return Prompts{}, fmt.Errorf("load %s: %w", ref, err)
		}
		*dst = rules + "\n\n---\n\n" + task
	}
	return p, nil
}

// Labeller is one model profile used to label or adjudicate.
type Labeller struct {
	Name   string // profile reference (recorded on the run)
	Model  string // model name for the call
	Client docprocessing.LLMJSONExtractor
}

// Activity is one labelled activity. Spans are inclusive line numbers.
type Activity struct {
	Name          string
	Object        string
	LineStart     int
	LineEnd       int
	DocumentLevel bool
	SourceText    string
	Agreement     string
	Rationale     string
}

// Candidate is one pooled search result.
type Candidate struct {
	ArtifactType string
	ArtifactID   string
	RecordID     int64
	Text         string
	Cosine       *float64
}

func (c Candidate) key() string { return c.ArtifactType + "/" + c.ArtifactID }

// Judgement is both annotators' labels for one candidate and the final label.
type Judgement struct {
	Candidate   Candidate
	Label1      string
	Reason1     string
	Label2      string
	Reason2     string
	Final       string
	Adjudicated bool
	Rationale   string
}

// GoldActivity is a final activity with its judged pool.
type GoldActivity struct {
	Activity
	Judgements []Judgement
}

// GoldDocument is the gold labelling of one document.
type GoldDocument struct {
	GoldDoc
	Activities []GoldActivity
	Dropped    int // activities proposed by one annotator only and rejected by the adjudicator
}

// Pipeline labels documents. Chunks, Exclude and Search are fields so tests need no database.
type Pipeline struct {
	A1, A2, Adj Labeller
	Prompts     Prompts
	PoolLimit   int
	// Scope is the snapshot's member records: the pool searches only these, so every gold label
	// refers to a frozen record. Empty means no restriction (tests only).
	Scope   []int64
	Chunks  func(ctx context.Context, recordID int64) ([]docreviews.GoldChunk, error)
	Exclude func(ctx context.Context, recordID int64) ([]int64, error)
	Search  func(ctx context.Context, req searchengine.Request) (*searchengine.Response, error)
}

func (p *Pipeline) call(ctx context.Context, lab Labeller, prompt, promptName, input, reason string) (map[string]any, error) {
	out, err := lab.Client.ExtractJSON(ctx, docprocessing.NewLLMJSONInput(ctx, promptName, prompt, lab.Model, input, reason, "MID-20261112-gold"))
	if err != nil {
		return nil, fmt.Errorf("%s (%s): %w", reason, lab.Name, err)
	}
	return out, nil
}

// LabelDocument runs the whole procedure for one document.
func (p *Pipeline) LabelDocument(ctx context.Context, doc GoldDoc) (*GoldDocument, error) {
	chunks, err := p.Chunks(ctx, doc.RecordID)
	if err != nil {
		return nil, err
	}
	acts1, err := p.extract(ctx, p.A1, chunks)
	if err != nil {
		return nil, fmt.Errorf("record %d: %w", doc.RecordID, err)
	}
	acts2, err := p.extract(ctx, p.A2, chunks)
	if err != nil {
		return nil, fmt.Errorf("record %d: %w", doc.RecordID, err)
	}
	final, dropped, err := p.merge(ctx, acts1, acts2)
	if err != nil {
		return nil, fmt.Errorf("record %d: %w", doc.RecordID, err)
	}
	exclude, err := p.Exclude(ctx, doc.RecordID)
	if err != nil {
		return nil, err
	}
	out := &GoldDocument{GoldDoc: doc, Dropped: dropped}
	for _, a := range final {
		pool, err := p.pool(ctx, a, exclude)
		if err != nil {
			return nil, fmt.Errorf("record %d activity %q: %w", doc.RecordID, a.Name, err)
		}
		js, err := p.judge(ctx, a, pool)
		if err != nil {
			return nil, fmt.Errorf("record %d activity %q: %w", doc.RecordID, a.Name, err)
		}
		out.Activities = append(out.Activities, GoldActivity{Activity: a, Judgements: js})
	}
	return out, nil
}

// extract asks one annotator for the activities of every chunk (and the document-level activity in chunk 0).
func (p *Pipeline) extract(ctx context.Context, lab Labeller, chunks []docreviews.GoldChunk) ([]Activity, error) {
	var out []Activity
	for _, ch := range chunks {
		in := fmt.Sprintf(`{"first_chunk":%t,"chunk":%s}`, ch.Index == 0, ch.InputJSON)
		resp, err := p.call(ctx, lab, p.Prompts.Activities, ActivitiesPromptFile, in, "compliance_gold_activities")
		if err != nil {
			return nil, err
		}
		raw, ok := resp["activities"].([]any)
		if !ok {
			return nil, fmt.Errorf("%s: response has no activities array", lab.Name)
		}
		add := func(m map[string]any, docLevel bool) error {
			a := Activity{
				Name: str(m, "activity"), Object: str(m, "object"),
				LineStart: num(m, "line_start"), LineEnd: num(m, "line_end"), DocumentLevel: docLevel,
			}
			if a.Name == "" || a.LineStart < ch.StartLine || a.LineEnd > ch.EndLine || a.LineEnd < a.LineStart {
				return fmt.Errorf("%s: invalid activity %v for chunk lines %d-%d", lab.Name, m, ch.StartLine, ch.EndLine)
			}
			a.SourceText = docreviews.GoldLinesText(ch.InputJSON, a.LineStart, a.LineEnd)
			out = append(out, a)
			return nil
		}
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s: activity entry is not an object", lab.Name)
			}
			if err := add(m, false); err != nil {
				return nil, err
			}
		}
		if da, ok := resp["document_activity"].(map[string]any); ok && ch.Index == 0 {
			if err := add(da, true); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func overlaps(a, b Activity) bool {
	if a.DocumentLevel != b.DocumentLevel {
		return false
	}
	lo, hi := max(a.LineStart, b.LineStart), min(a.LineEnd, b.LineEnd)
	if hi < lo {
		return false
	}
	shorter := min(a.LineEnd-a.LineStart, b.LineEnd-b.LineStart) + 1
	return float64(hi-lo+1)/float64(shorter) >= overlapThreshold
}

// merge keeps activities both annotators found (matched by span overlap) and sends the rest to the
// adjudicator. The result is ordered by line span; document-level first.
func (p *Pipeline) merge(ctx context.Context, a1, a2 []Activity) (final []Activity, dropped int, err error) {
	used2 := make([]bool, len(a2))
	var single []struct {
		a  Activity
		by string
	}
	for _, x := range a1 {
		matched := false
		for j, y := range a2 {
			if !used2[j] && overlaps(x, y) {
				used2[j], matched = true, true
				u := x
				u.LineStart, u.LineEnd = min(x.LineStart, y.LineStart), max(x.LineEnd, y.LineEnd)
				u.SourceText = x.SourceText
				if y.LineStart < x.LineStart || y.LineEnd > x.LineEnd {
					u.SourceText = x.SourceText + "\n" + y.SourceText // wider span: keep both quotes
				}
				u.Agreement = agreeBoth
				final = append(final, u)
				break
			}
		}
		if !matched {
			single = append(single, struct {
				a  Activity
				by string
			}{x, "annotator_1"})
		}
	}
	for j, y := range a2 {
		if !used2[j] {
			single = append(single, struct {
				a  Activity
				by string
			}{y, "annotator_2"})
		}
	}
	for _, s := range single {
		in, _ := json.Marshal(map[string]any{"activity": map[string]any{
			"activity": s.a.Name, "object": s.a.Object, "line_start": s.a.LineStart, "line_end": s.a.LineEnd,
			"source_text": truncate(s.a.SourceText, activityTextMax)}, "proposed_by": s.by})
		resp, err := p.call(ctx, p.Adj, p.Prompts.AdjActivity, AdjActivityPromptFile, string(in), "compliance_gold_adjudicate_activity")
		if err != nil {
			return nil, 0, err
		}
		keep, ok := resp["keep"].(bool)
		if !ok {
			return nil, 0, fmt.Errorf("adjudicator: response has no boolean keep")
		}
		if !keep {
			dropped++
			continue
		}
		s.a.Agreement, s.a.Rationale = agreeAdjudicated, str(resp, "rationale")
		final = append(final, s.a)
	}
	sort.SliceStable(final, func(i, j int) bool {
		if final[i].DocumentLevel != final[j].DocumentLevel {
			return final[i].DocumentLevel
		}
		return final[i].LineStart < final[j].LineStart
	})
	return final, dropped, nil
}

// pool searches the snapshot corpus deeply for one activity: no relevance floor, the document and its
// byte-identical copies excluded, duplicates across queries merged keeping the highest cosine.
func (p *Pipeline) pool(ctx context.Context, a Activity, exclude []int64) ([]Candidate, error) {
	queries := []string{truncate(a.SourceText, activityTextMax), strings.TrimSpace(strings.ReplaceAll(a.Name, "_", " ") + " " + a.Object)}
	byKey := map[string]*Candidate{}
	var order []string
	for _, q := range queries {
		if q == "" {
			continue
		}
		resp, err := p.Search(ctx, searchengine.Request{
			Query:   searchengine.Query{Text: q},
			Filters: searchengine.Filters{ArtifactTypes: []string{"provision", "chunk"}, RecordIDs: p.Scope, ExcludeRecordIDs: exclude},
			Options: searchengine.Options{Limit: p.PoolLimit},
		})
		if err != nil {
			return nil, fmt.Errorf("pool search %q: %w", truncate(q, 60), err)
		}
		for _, r := range resp.Results {
			c := Candidate{ArtifactType: r.ArtifactType, ArtifactID: r.ArtifactID, RecordID: r.InputRecordID,
				Text: truncate(strings.TrimSpace(r.Snippet), candidateTextMax), Cosine: r.Diag.Cosine}
			if prev, ok := byKey[c.key()]; ok {
				if c.Cosine != nil && (prev.Cosine == nil || *c.Cosine > *prev.Cosine) {
					prev.Cosine = c.Cosine
				}
				continue
			}
			byKey[c.key()] = &c
			order = append(order, c.key())
		}
	}
	out := make([]Candidate, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out, nil
}

type annotatorLabel struct{ label, reason string }

// judgeAttempts is how many times an annotator is asked for the labels still missing in a batch.
const judgeAttempts = 3

// judgeBy asks one annotator to label every candidate, in batches. A label that is missing, repeated,
// for an unknown id or outside the vocabulary does not count; only the candidates still unlabelled
// are asked again, up to judgeAttempts times, then the run fails. A silent gap would bias the gold set.
func (p *Pipeline) judgeBy(ctx context.Context, lab Labeller, a Activity, pool []Candidate) (map[string]annotatorLabel, error) {
	out := map[string]annotatorLabel{}
	for i := 0; i < len(pool); i += judgeBatch {
		pending := pool[i:min(i+judgeBatch, len(pool))]
		for attempt := 1; len(pending) > 0; attempt++ {
			type cand struct {
				ID   string `json:"id"`
				Text string `json:"text"`
			}
			cs := make([]cand, len(pending))
			want := map[string]bool{}
			for j, c := range pending {
				cs[j] = cand{c.key(), c.Text}
				want[c.key()] = true
			}
			in, _ := json.Marshal(map[string]any{"activity": map[string]any{"activity": a.Name, "object": a.Object,
				"source_text": truncate(a.SourceText, activityTextMax)}, "candidates": cs})
			resp, err := p.call(ctx, lab, p.Prompts.Judge, JudgePromptFile, string(in), "compliance_gold_judge")
			if err != nil {
				return nil, err
			}
			labels, ok := resp["labels"].([]any)
			if !ok {
				return nil, fmt.Errorf("%s: response has no labels array", lab.Name)
			}
			for _, item := range labels {
				m, _ := item.(map[string]any)
				id, label := str(m, "id"), str(m, "label")
				if !want[id] || (label != labelGoverns && label != labelTopical && label != labelUnrelated) {
					continue
				}
				delete(want, id)
				out[id] = annotatorLabel{label, str(m, "reason")}
			}
			var still []Candidate
			for _, c := range pending {
				if want[c.key()] {
					still = append(still, c)
				}
			}
			pending = still
			if len(pending) > 0 && attempt >= judgeAttempts {
				return nil, fmt.Errorf("%s: no valid label for %d candidate(s) of activity %q after %d attempts", lab.Name, len(pending), a.Name, judgeAttempts)
			}
		}
	}
	return out, nil
}

// judge has both annotators label the pool and the adjudicator settle every disagreement.
func (p *Pipeline) judge(ctx context.Context, a Activity, pool []Candidate) ([]Judgement, error) {
	if len(pool) == 0 {
		return nil, nil
	}
	l1, err := p.judgeBy(ctx, p.A1, a, pool)
	if err != nil {
		return nil, err
	}
	l2, err := p.judgeBy(ctx, p.A2, a, pool)
	if err != nil {
		return nil, err
	}
	out := make([]Judgement, 0, len(pool))
	for _, c := range pool {
		x, y := l1[c.key()], l2[c.key()]
		j := Judgement{Candidate: c, Label1: x.label, Reason1: x.reason, Label2: y.label, Reason2: y.reason, Final: x.label}
		if x.label != y.label {
			in, _ := json.Marshal(map[string]any{
				"activity":    map[string]any{"activity": a.Name, "object": a.Object, "source_text": truncate(a.SourceText, activityTextMax)},
				"candidate":   map[string]any{"id": c.key(), "text": c.Text},
				"annotator_1": map[string]any{"label": x.label, "reason": x.reason},
				"annotator_2": map[string]any{"label": y.label, "reason": y.reason},
			})
			resp, err := p.call(ctx, p.Adj, p.Prompts.AdjJudgement, AdjJudgementPromptFile, string(in), "compliance_gold_adjudicate_judgement")
			if err != nil {
				return nil, err
			}
			label := str(resp, "label")
			switch label {
			case labelGoverns, labelTopical, labelUnrelated, labelAmbiguous:
			default:
				return nil, fmt.Errorf("adjudicator: label %q for candidate %q is not valid", label, c.key())
			}
			j.Final, j.Adjudicated, j.Rationale = label, true, str(resp, "rationale")
		}
		out = append(out, j)
	}
	return out, nil
}

func str(m map[string]any, k string) string { s, _ := m[k].(string); return strings.TrimSpace(s) }

func num(m map[string]any, k string) int {
	if f, ok := m[k].(float64); ok {
		return int(f)
	}
	return 0
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
