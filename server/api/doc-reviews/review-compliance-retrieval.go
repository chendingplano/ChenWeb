package docreviews

import (
	"context"
	"fmt"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

// complianceCandidateTextMax caps each candidate's indexed text in the model input.
const complianceCandidateTextMax = 2000

var complianceTextActivityKinds = []string{"provision", "metric", "chunk"}

// complianceCandidate is one knowledge-base artifact found for an activity.
type complianceCandidate struct {
	Key       string   `json:"artifact_key"`
	Type      string   `json:"type"`
	ID        string   `json:"-"`
	RecordID  int64    `json:"record_id"`
	Title     string   `json:"title,omitempty"`
	Text      string   `json:"text"`
	Truncated bool     `json:"truncated,omitempty"`
	Cosine    *float64 `json:"-"`
	LineStart int      `json:"-"`
	LineEnd   int      `json:"-"`
}

// complianceTraceResult is one search result as the trace keeps it: which artifact, its ranks and
// scores. The text is left out; a judged candidate's text is in the finding.
type complianceTraceResult struct {
	ArtifactType string                   `json:"artifact_type"`
	ArtifactID   string                   `json:"artifact_id"`
	RecordID     int64                    `json:"record_id"`
	Title        string                   `json:"title,omitempty"`
	Rank         int                      `json:"rank"`
	Score        float64                  `json:"score"`
	Diag         searchengine.Diagnostics `json:"diag"`
}

// complianceRetrievalTrace records one search: what was asked, under which limits, and what came back.
type complianceRetrievalTrace struct {
	ActivityID           string                  `json:"activity_id"`
	Purpose              string                  `json:"purpose"` // activity, cited_document or candidate_reference
	Query                string                  `json:"query,omitempty"`
	Reference            string                  `json:"reference,omitempty"`
	CitedDocument        string                  `json:"cited_document,omitempty"`
	Keywords             []string                `json:"keywords,omitempty"`
	ArtifactTypes        []string                `json:"artifact_types"`
	RecordIDs            []int64                 `json:"record_ids,omitempty"`
	ExcludedRecordIDs    []int64                 `json:"excluded_record_ids"`
	Limit                int                     `json:"limit"`
	PotentiallyTruncated bool                    `json:"potentially_truncated"`
	Results              []complianceTraceResult `json:"results"`
}

// complianceMemberSearch is what the search found for one activity.
type complianceMemberSearch struct {
	Traces []complianceRetrievalTrace
	Found  []string // candidate keys in rank order
}

// memberKinds returns the artifact kinds an activity searches and how many results it keeps.
func (r *complianceReviewer) memberKinds(m *complianceMember) ([]string, int) {
	switch m.Kind {
	case "metric":
		return []string{"metric"}, r.settings.TopMetrics
	case "provision":
		return []string{"provision"}, r.settings.TopProvisions
	default:
		return complianceTextActivityKinds, r.settings.TopArtifacts
	}
}

// searchMember searches the knowledge base for one activity, adds what it finds to pool, and
// searches the activity's cited documents and its candidates' references within
// max_cited_documents.
func (r *complianceReviewer) searchMember(ctx context.Context, exclude []int64, m *complianceMember, pool map[string]*complianceCandidate) (complianceMemberSearch, error) {
	var out complianceMemberSearch
	kinds, limit := r.memberKinds(m)
	seen := map[string]bool{}
	add := func(results []searchengine.Result) {
		for _, res := range results {
			key := res.ArtifactType + "/" + res.ArtifactID
			if _, ok := pool[key]; !ok {
				pool[key] = complianceCandidateFrom(res)
			}
			if !seen[key] {
				seen[key] = true
				out.Found = append(out.Found, key)
			}
		}
	}
	run := func(purpose, cited string, req searchengine.Request) ([]searchengine.Result, error) {
		req.Filters.ArtifactTypes = kinds
		req.Filters.ExcludeRecordIDs = exclude
		req.Options.Limit = limit
		resp, err := r.search(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("search for %s %q: %w", m.Kind, m.Label, err)
		}
		if resp == nil {
			return nil, fmt.Errorf("search for %s %q returned no response", m.Kind, m.Label)
		}
		t := complianceRetrievalTrace{ActivityID: m.ID, Purpose: purpose, Query: req.Query.Text, CitedDocument: cited, Keywords: resp.Keywords,
			ArtifactTypes: kinds, RecordIDs: req.Filters.RecordIDs, ExcludedRecordIDs: exclude, Limit: limit,
			PotentiallyTruncated: len(resp.Results) >= limit, Results: complianceTraceResults(resp.Results)}
		if req.Query.Ref != nil {
			t.Reference = req.Query.Ref.Type + "/" + req.Query.Ref.ID
		}
		out.Traces = append(out.Traces, t)
		return resp.Results, nil
	}

	// The activity's own search.
	switch m.Kind {
	case "metric", "provision":
		if m.ArtifactID == "" {
			return out, fmt.Errorf("%s %q (row %d) is not in the search index, so it cannot be searched", m.Kind, m.Label, m.RowID)
		}
		res, err := run("activity", "", r.memberRequest(m, nil))
		if err != nil {
			return out, err
		}
		add(res)
	default:
		var lists [][]searchengine.Result
		for _, q := range complianceMemberQueries(m) {
			res, err := run("activity", "", searchengine.Request{Query: searchengine.Query{Text: q.Text}, Criteria: searchengine.Criteria{Keywords: q.Keywords}})
			if err != nil {
				return out, err
			}
			lists = append(lists, res)
		}
		add(complianceRoundRobin(lists, limit))
	}

	// Cited documents: the activity's own first, then the documents its candidates depend on.
	budget := r.settings.MaxCitedDocuments
	if budget <= 0 {
		return out, nil
	}
	docs := append([]complianceCitedDoc(nil), m.CitedDocs...)
	if len(docs) < budget {
		refs, err := r.candidateReferences(ctx, out.Found, pool)
		if err != nil {
			return out, err
		}
		docs = append(docs, refs...)
	}
	searched := map[string]bool{}
	for _, d := range docs {
		if len(searched) >= budget {
			break
		}
		key := strings.ToLower(d.Name)
		if d.RecordID > 0 {
			key = fmt.Sprintf("record:%d", d.RecordID)
		}
		if searched[key] {
			continue
		}
		searched[key] = true
		purpose := "cited_document"
		if !complianceCitedBy(m, d) {
			purpose = "candidate_reference"
		}
		var req searchengine.Request
		if d.RecordID > 0 {
			// The cited record is known: search it for this activity.
			req = r.memberRequest(m, []int64{d.RecordID})
		} else {
			req = searchengine.Request{Query: searchengine.Query{Text: d.Name}}
		}
		res, err := run(purpose, d.Name, req)
		if err != nil {
			return out, err
		}
		add(res)
	}
	return out, nil
}

// memberRequest is the activity's own search request, optionally limited to some records.
func (r *complianceReviewer) memberRequest(m *complianceMember, records []int64) searchengine.Request {
	req := searchengine.Request{Filters: searchengine.Filters{RecordIDs: records}}
	if m.ArtifactID != "" && (m.Kind == "metric" || m.Kind == "provision") {
		req.Query.Ref = &searchengine.ArtifactRef{Type: m.Kind, ID: m.ArtifactID}
		return req
	}
	qs := complianceMemberQueries(m)
	req.Query.Text = qs[0].Text
	req.Criteria.Keywords = qs[0].Keywords
	return req
}

// complianceMemberQueries returns a text activity's searches, or its quote when it has none.
func complianceMemberQueries(m *complianceMember) []complianceQuery {
	if m.Activity != nil && len(m.Activity.Queries) > 0 {
		return m.Activity.Queries
	}
	return []complianceQuery{{Text: m.Text}}
}

func complianceCitedBy(m *complianceMember, d complianceCitedDoc) bool {
	for _, c := range m.CitedDocs {
		if c == d {
			return true
		}
	}
	return false
}

// candidateReferences returns the external documents the found candidates cite on their own lines,
// as reference extraction matched them.
func (r *complianceReviewer) candidateReferences(ctx context.Context, found []string, pool map[string]*complianceCandidate) ([]complianceCitedDoc, error) {
	var out []complianceCitedDoc
	for _, key := range found {
		c := pool[key]
		if c == nil || c.LineStart == 0 {
			continue
		}
		mentions, err := r.recordMentions(ctx, c.RecordID)
		if err != nil {
			return nil, err
		}
		for _, d := range complianceMentionsIn(mentions, c.LineStart, c.LineEnd) {
			if d.RecordID > 0 {
				out = append(out, d)
			}
		}
	}
	return out, nil
}

func complianceTraceResults(results []searchengine.Result) []complianceTraceResult {
	out := make([]complianceTraceResult, 0, len(results))
	for _, res := range results {
		title := res.SourceTitle
		if title == "" {
			title = res.SourceFilename
		}
		out = append(out, complianceTraceResult{ArtifactType: res.ArtifactType, ArtifactID: res.ArtifactID, RecordID: res.InputRecordID,
			Title: title, Rank: res.Rank, Score: res.Score, Diag: res.Diag})
	}
	return out
}

// complianceRoundRobin takes the lists' results in turn, best first, until limit distinct results.
func complianceRoundRobin(lists [][]searchengine.Result, limit int) []searchengine.Result {
	var out []searchengine.Result
	seen := map[string]bool{}
	for rank := 0; len(out) < limit; rank++ {
		more := false
		for _, list := range lists {
			if rank >= len(list) {
				continue
			}
			more = true
			key := list[rank].ArtifactType + "/" + list[rank].ArtifactID
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, list[rank])
			if len(out) == limit {
				break
			}
		}
		if !more {
			break
		}
	}
	return out
}

func complianceCandidateFrom(res searchengine.Result) *complianceCandidate {
	title := res.SourceTitle
	if title == "" {
		title = res.SourceFilename
	}
	start, end := complianceSpanRange(res.SourceLineSpans)
	c := &complianceCandidate{Key: res.ArtifactType + "/" + res.ArtifactID, Type: res.ArtifactType, ID: res.ArtifactID, RecordID: res.InputRecordID,
		Title: title, Cosine: res.Diag.Cosine, LineStart: start, LineEnd: end}
	c.setText(res.Snippet)
	return c
}

func (c *complianceCandidate) setText(text string) {
	text = strings.TrimSpace(text)
	c.Truncated = false
	if runes := []rune(text); len(runes) > complianceCandidateTextMax {
		text, c.Truncated = string(runes[:complianceCandidateTextMax]), true
	}
	c.Text = text
}

// loadCandidateTexts replaces the search snippets with the full text the index stores for each
// candidate (cut to complianceCandidateTextMax).
func (r *complianceReviewer) loadCandidateTexts(ctx context.Context, pool map[string]*complianceCandidate) error {
	if r.loadArtifacts == nil || len(pool) == 0 {
		return nil
	}
	refs := make([]searchengine.ArtifactRef, 0, len(pool))
	for _, c := range pool {
		refs = append(refs, searchengine.ArtifactRef{Type: c.Type, ID: c.ID})
	}
	loaded, err := r.loadArtifacts(ctx, refs)
	if err != nil {
		return fmt.Errorf("load candidate text: %w", err)
	}
	for _, res := range loaded {
		if c := pool[res.ArtifactType+"/"+res.ArtifactID]; c != nil && strings.TrimSpace(res.Snippet) != "" {
			c.setText(res.Snippet)
		}
	}
	return nil
}
