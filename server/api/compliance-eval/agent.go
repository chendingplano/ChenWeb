package complianceeval

// Agent-built gold, the first iteration's labelling (openspec compliance-reviewer-phase2, design D4
// and D7 revised 2026-11-14). The coding agent follows the compliance-review-benchmark skill:
//
//  1. `compliance-gold chunks` prints the document as the reviewer reads it, chunk by chunk;
//  2. the agent writes an activities file (ActivitiesFile);
//  3. `compliance-gold pool` checks it (CheckActivities) and writes a pool file (PoolFile): for each
//     activity the candidates pooled from the whole knowledge base;
//  4. the agent writes one label per pooled candidate, one JSON object per line (AgentLabel);
//  5. `compliance-gold expand` searches within every record with a governs label (ExpandPool), and
//     the agent labels what it adds, until it adds nothing (design D11);
//  6. `compliance-gold save` checks every label (BuildAgentDocument) and saves the run.
//
// `compliance-gold augment` later adds the passages a reviewer run reported that the run never
// judged (AugmentPool, design D12); saving their labels makes a new run.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	docreviews "github.com/chendingplano/deepdoc/server/api/doc-reviews"
)

// AgentActivity is one activity as the agent writes it. Queries are extra pool queries, searched as
// written; CitedDocs are the documents the text cites for the activity (design D11).
type AgentActivity struct {
	Activity      string   `json:"activity"`
	Object        string   `json:"object"`
	LineStart     int      `json:"line_start"`
	LineEnd       int      `json:"line_end"`
	DocumentLevel bool     `json:"document_level"`
	Queries       []string `json:"queries,omitempty"`
	CitedDocs     []string `json:"cited_docs,omitempty"`
}

// cleanList trims the entries, refusing an empty one, a repeated one, or more than max.
func cleanList(in []string, max int, what string) ([]string, error) {
	if len(in) > max {
		return nil, fmt.Errorf("%d %s, at most %d are allowed", len(in), what, max)
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		switch {
		case s == "":
			return nil, fmt.Errorf("an empty entry in %s", what)
		case seen[s]:
			return nil, fmt.Errorf("%q is listed twice in %s", s, what)
		}
		seen[s] = true
		out = append(out, s)
	}
	return out, nil
}

// ActivitiesFile is the agent's activities file for one document.
type ActivitiesFile struct {
	RecordID   int64           `json:"record_id"`
	Activities []AgentActivity `json:"activities"`
}

// CheckActivities validates the agent's activities against the document's chunks and returns them
// with their source text. Each span lies inside one chunk, the reviewer's unit; the document-level
// activity, at most one, lies in the first chunk; the same activity twice is an error.
func CheckActivities(chunks []docreviews.GoldChunk, in []AgentActivity) ([]Activity, error) {
	if len(chunks) == 0 {
		return nil, fmt.Errorf("the document has no chunks")
	}
	seen := map[string]bool{}
	docLevel := 0
	out := make([]Activity, 0, len(in))
	for i, a := range in {
		name := strings.TrimSpace(a.Activity)
		where := fmt.Sprintf("activity %d (%q, lines %d-%d)", i+1, name, a.LineStart, a.LineEnd)
		if name == "" {
			return nil, fmt.Errorf("activity %d: the activity name is empty", i+1)
		}
		if a.LineEnd < a.LineStart {
			return nil, fmt.Errorf("%s: line_end is before line_start", where)
		}
		var ch *docreviews.GoldChunk
		for j := range chunks {
			if a.LineStart >= chunks[j].StartLine && a.LineEnd <= chunks[j].EndLine {
				ch = &chunks[j]
				break
			}
		}
		if ch == nil {
			return nil, fmt.Errorf("%s: the span is not inside one chunk (compliance-gold chunks lists them)", where)
		}
		if a.DocumentLevel {
			if docLevel++; docLevel > 1 {
				return nil, fmt.Errorf("%s: only one document-level activity is allowed", where)
			}
			if ch.Index != 0 {
				return nil, fmt.Errorf("%s: the document-level activity must lie in the first chunk", where)
			}
		}
		key := fmt.Sprintf("%s|%d|%d|%t", name, a.LineStart, a.LineEnd, a.DocumentLevel)
		if seen[key] {
			return nil, fmt.Errorf("%s: listed twice", where)
		}
		seen[key] = true
		text := docreviews.GoldLinesText(ch.InputJSON, a.LineStart, a.LineEnd)
		if text == "" {
			return nil, fmt.Errorf("%s: those lines hold no text", where)
		}
		queries, err := cleanList(a.Queries, MaxAgentQueries, "queries")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", where, err)
		}
		cited, err := cleanList(a.CitedDocs, MaxCitedDocs, "cited_docs")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", where, err)
		}
		out = append(out, Activity{Name: name, Object: strings.TrimSpace(a.Object), LineStart: a.LineStart,
			LineEnd: a.LineEnd, DocumentLevel: a.DocumentLevel, SourceText: text, Agreement: agreeSingle,
			Queries: queries, CitedDocs: cited})
	}
	return out, nil
}

// PoolFile is what `compliance-gold pool`, `expand` and `augment` write and `save` reads. An
// augmentation pool names its BaseRun and ReviewRun and holds only the passages to add.
type PoolFile struct {
	SnapshotID    int64 `json:"snapshot_id"`
	RecordID      int64 `json:"record_id"`
	MaxCandidates int   `json:"max_candidates"`
	// ExcludedRecords are the document and its byte-identical copies.
	ExcludedRecords []int64 `json:"excluded_records"`
	// AddedSinceSnapshot counts records left out because they were indexed after the snapshot.
	AddedSinceSnapshot int            `json:"added_since_snapshot"`
	BaseRun            string         `json:"base_run,omitempty"`
	ReviewRun          int64          `json:"review_run,omitempty"`
	Activities         []PoolActivity `json:"activities"`
}

// PoolActivity is one activity with its pooled candidates. ID is a1, a2, ... in file order.
// SearchedRecords are the records searched within for it (cited documents and expansion);
// UnresolvedCitedDocs are citations no usable record holds.
type PoolActivity struct {
	ID string `json:"id"`
	AgentActivity
	SourceText          string          `json:"source_text"`
	SearchedRecords     []int64         `json:"searched_records,omitempty"`
	UnresolvedCitedDocs []string        `json:"unresolved_cited_docs,omitempty"`
	Candidates          []PoolCandidate `json:"candidates"`
}

// PoolCandidate is one pooled passage. ID is <artifact_type>/<artifact_id>; FoundBy names the list
// that first found it.
type PoolCandidate struct {
	ID           string   `json:"id"`
	ArtifactType string   `json:"artifact_type"`
	ArtifactID   string   `json:"artifact_id"`
	RecordID     int64    `json:"record_id"`
	Source       string   `json:"source"`
	Cosine       *float64 `json:"cosine"`
	FoundBy      string   `json:"found_by"`
	Text         string   `json:"text"`
}

func poolCandidate(c Candidate) PoolCandidate {
	return PoolCandidate{ID: c.key(), ArtifactType: c.ArtifactType, ArtifactID: c.ArtifactID, RecordID: c.RecordID,
		Source: c.Source, Cosine: c.Cosine, FoundBy: c.FoundBy, Text: c.Text}
}

func agentActivityOf(a Activity) AgentActivity {
	return AgentActivity{Activity: a.Name, Object: a.Object, LineStart: a.LineStart, LineEnd: a.LineEnd,
		DocumentLevel: a.DocumentLevel, Queries: a.Queries, CitedDocs: a.CitedDocs}
}

// searchWithin adds up to WithinPerRecord new passages of record to pa, unless pa already searched it.
func searchWithin(ctx context.Context, p *Pipeline, a Activity, pa *PoolActivity, record int64, foundBy string) (int, error) {
	for _, r := range pa.SearchedRecords {
		if r == record {
			return 0, nil
		}
	}
	skip := map[string]bool{}
	for _, c := range pa.Candidates {
		skip[c.ID] = true
	}
	cands, err := p.Within(ctx, a, record, skip, foundBy)
	if err != nil {
		return 0, fmt.Errorf("activity %s (%q) record %d: %w", pa.ID, a.Name, record, err)
	}
	for _, c := range cands {
		pa.Candidates = append(pa.Candidates, poolCandidate(c))
	}
	pa.SearchedRecords = append(pa.SearchedRecords, record)
	return len(cands), nil
}

// PoolActivities pools candidates for every checked activity (design D11): the capped search of the
// knowledge base, then a search within every record cites resolves for the activity's cited
// documents. cites must index only records the pool may use.
func PoolActivities(ctx context.Context, p *Pipeline, acts []Activity, exclude []int64, cites CitationIndex) ([]PoolActivity, error) {
	out := make([]PoolActivity, 0, len(acts))
	for i, a := range acts {
		cands, err := p.Pool(ctx, a, exclude)
		if err != nil {
			return nil, fmt.Errorf("activity %q: %w", a.Name, err)
		}
		pa := PoolActivity{ID: fmt.Sprintf("a%d", i+1), SourceText: a.SourceText, Candidates: []PoolCandidate{},
			AgentActivity: agentActivityOf(a)}
		for _, c := range cands {
			pa.Candidates = append(pa.Candidates, poolCandidate(c))
		}
		for _, cite := range a.CitedDocs {
			recs := cites.Resolve(cite)
			if len(recs) == 0 {
				pa.UnresolvedCitedDocs = append(pa.UnresolvedCitedDocs, cite)
				continue
			}
			for _, rec := range recs {
				if _, err := searchWithin(ctx, p, a, &pa, rec, "cited:"+cite); err != nil {
					return nil, err
				}
			}
		}
		out = append(out, pa)
	}
	return out, nil
}

// labelPairs checks that every label names a pooled pair and indexes the labels by pair.
func labelPairs(pf PoolFile, labels []AgentLabel) (map[string]AgentLabel, error) {
	pooled := map[string]bool{}
	for _, pa := range pf.Activities {
		for _, c := range pa.Candidates {
			pooled[pairKey(pa.ID, c.ID)] = true
		}
	}
	byPair := map[string]AgentLabel{}
	for i, l := range labels {
		k := pairKey(strings.TrimSpace(l.Activity), strings.TrimSpace(l.Candidate))
		switch {
		case !pooled[k]:
			return nil, fmt.Errorf("label %d: activity %q has no pooled candidate %q", i+1, l.Activity, l.Candidate)
		case byPair[k] != (AgentLabel{}):
			return nil, fmt.Errorf("label %d: activity %q candidate %q is labelled twice", i+1, l.Activity, l.Candidate)
		}
		byPair[k] = l
	}
	return byPair, nil
}

func pairKey(a, c string) string { return a + " " + c }

// unexpanded lists, per activity, the records with a governs label that were not searched within.
func unexpanded(pf PoolFile, byPair map[string]AgentLabel) []string {
	var out []string
	for _, pa := range pf.Activities {
		searched := map[int64]bool{}
		for _, r := range pa.SearchedRecords {
			searched[r] = true
		}
		for _, c := range pa.Candidates {
			if byPair[pairKey(pa.ID, c.ID)].Label == labelGoverns && !searched[c.RecordID] {
				searched[c.RecordID] = true // report each record once per activity
				out = append(out, fmt.Sprintf("%s record %d", pa.ID, c.RecordID))
			}
		}
	}
	return out
}

// ExpandPool searches, for each activity, within every record with a governs label that was not yet
// searched for it, and adds what it finds to pf (design D11). acts are the checked activities in
// pool-file order. It returns how many passages were added; zero means expansion is complete.
func ExpandPool(ctx context.Context, p *Pipeline, pf *PoolFile, acts []Activity, labels []AgentLabel) (int, error) {
	if pf.BaseRun != "" {
		return 0, fmt.Errorf("an augmentation pool is not expanded")
	}
	if len(acts) != len(pf.Activities) {
		return 0, fmt.Errorf("pool file has %d activities, %d were checked", len(pf.Activities), len(acts))
	}
	byPair, err := labelPairs(*pf, labels)
	if err != nil {
		return 0, err
	}
	added := 0
	for i := range pf.Activities {
		pa := &pf.Activities[i]
		var governing []int64
		seen := map[int64]bool{}
		for _, c := range pa.Candidates {
			if byPair[pairKey(pa.ID, c.ID)].Label == labelGoverns && !seen[c.RecordID] {
				seen[c.RecordID] = true
				governing = append(governing, c.RecordID)
			}
		}
		for _, rec := range governing {
			n, err := searchWithin(ctx, p, acts[i], pa, rec, fmt.Sprintf("expand:%d", rec))
			if err != nil {
				return added, err
			}
			added += n
		}
	}
	return added, nil
}

// AgentLabel is one judgement the agent writes: the activity id and candidate id from the pool file,
// a label (governs, topical, unrelated or ambiguous) and a reason.
type AgentLabel struct {
	Activity  string `json:"activity"`
	Candidate string `json:"candidate"`
	Label     string `json:"label"`
	Reason    string `json:"reason"`
}

// ParseLabels reads the labels file: one JSON object per line, blank lines ignored. A line that is
// not a label object, or has a field AgentLabel does not know (a typo), is an error naming the line.
func ParseLabels(r io.Reader) ([]AgentLabel, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var out []AgentLabel
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		var l AgentLabel
		if err := dec.Decode(&l); err != nil {
			return nil, fmt.Errorf("labels line %d: %w", n, err)
		}
		out = append(out, l)
	}
	return out, sc.Err()
}

// BuildAgentDocument joins the checked activities (in pool-file order) with the agent's labels.
// Every pooled candidate needs exactly one label; a label for a pair the pool does not hold, a label
// outside the vocabulary, or a governs or ambiguous label without a reason is an error. In a pool
// that is not an augmentation, every record with a governs label must have been searched within
// (`compliance-gold expand`, design D11).
func BuildAgentDocument(pf PoolFile, acts []Activity, labels []AgentLabel, doc GoldDoc) (*GoldDocument, error) {
	if len(acts) != len(pf.Activities) {
		return nil, fmt.Errorf("pool file has %d activities, %d were checked", len(pf.Activities), len(acts))
	}
	byPair, err := labelPairs(pf, labels)
	if err != nil {
		return nil, err
	}
	for i, l := range labels {
		switch l.Label {
		case labelGoverns, labelAmbiguous:
			if strings.TrimSpace(l.Reason) == "" {
				return nil, fmt.Errorf("label %d: a %s label needs a reason (activity %q candidate %q)", i+1, l.Label, l.Activity, l.Candidate)
			}
		case labelTopical, labelUnrelated:
		default:
			return nil, fmt.Errorf("label %d: label %q is not governs, topical, unrelated or ambiguous", i+1, l.Label)
		}
	}
	var missing []string
	out := &GoldDocument{GoldDoc: doc}
	for i, pa := range pf.Activities {
		ga := GoldActivity{Activity: acts[i]}
		for _, c := range pa.Candidates {
			l, ok := byPair[pairKey(pa.ID, c.ID)]
			if !ok {
				missing = append(missing, pa.ID+" "+c.ID)
				continue
			}
			ga.Judgements = append(ga.Judgements, Judgement{
				Candidate: Candidate{ArtifactType: c.ArtifactType, ArtifactID: c.ArtifactID, RecordID: c.RecordID, Source: c.Source,
					Text: c.Text, Cosine: c.Cosine, FoundBy: c.FoundBy},
				Label1: l.Label, Reason1: strings.TrimSpace(l.Reason), Final: l.Label,
			})
		}
		out.Activities = append(out.Activities, ga)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		shown := missing[:min(len(missing), 5)]
		return nil, fmt.Errorf("%d pooled candidate(s) have no label, for example %s", len(missing), strings.Join(shown, ", "))
	}
	if pf.BaseRun == "" {
		if todo := unexpanded(pf, byPair); len(todo) > 0 {
			return nil, fmt.Errorf("%d record(s) with a governs label were not searched within; run compliance-gold expand and label what it adds: %s",
				len(todo), strings.Join(todo[:min(len(todo), 5)], ", "))
		}
	}
	return out, nil
}

// RulesRef names the frozen rules version a run used.
type RulesRef struct {
	File    string // e.g. compliance-review-benchmark/rules/1.0.0.md
	Version string
	SHA256  string
}

// LoadRulesVersion finds version in <rulesDir>/versions.json and checks that the rules file still
// has the SHA-256 recorded there: a released rules file is frozen, and an edited one is refused.
func LoadRulesVersion(rulesDir, version string) (RulesRef, error) {
	abs, err := filepath.Abs(rulesDir)
	if err != nil {
		return RulesRef{}, err
	}
	raw, err := os.ReadFile(filepath.Join(abs, "versions.json"))
	if err != nil {
		return RulesRef{}, fmt.Errorf("read rules versions: %w", err)
	}
	var vs struct {
		Versions map[string]struct {
			Rules  string `json:"rules"`
			SHA256 string `json:"sha256"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(raw, &vs); err != nil {
		return RulesRef{}, fmt.Errorf("parse %s: %w", filepath.Join(abs, "versions.json"), err)
	}
	v, ok := vs.Versions[version]
	if !ok {
		known := make([]string, 0, len(vs.Versions))
		for k := range vs.Versions {
			known = append(known, k)
		}
		sort.Strings(known)
		return RulesRef{}, fmt.Errorf("rules version %q is not released (known: %s)", version, strings.Join(known, ", "))
	}
	text, err := os.ReadFile(filepath.Join(abs, v.Rules))
	if err != nil {
		return RulesRef{}, fmt.Errorf("read rules %s: %w", v.Rules, err)
	}
	sum := sha256.Sum256(text)
	got := hex.EncodeToString(sum[:])
	if got != v.SHA256 {
		return RulesRef{}, fmt.Errorf("rules file %s was edited after release (sha256 %s, versions.json records %s); release a new version instead", v.Rules, got, v.SHA256)
	}
	return RulesRef{File: filepath.ToSlash(filepath.Join(filepath.Base(filepath.Dir(abs)), filepath.Base(abs), v.Rules)), Version: version, SHA256: got}, nil
}
