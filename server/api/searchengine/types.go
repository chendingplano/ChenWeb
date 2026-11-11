// Package searchengine is the knowledge base's hybrid search (keyword list + vector list, fused by
// reciprocal rank) as a Go API for server modules. It imports neither kbhandler nor doc-processing.
// Contract: openspec change hybrid-search-engine-api (specs/hybrid-search-engine).
package searchengine

import (
	"encoding/json"
	"errors"
	"strings"
)

// ArtifactRef names a row of kb.search_artifacts.
type ArtifactRef struct {
	Type string
	ID   string
}

// Query carries exactly one of a text (the engine embeds it) or a reference to an indexed artifact
// (the engine uses its stored text, embedding and keywords). Callers never pass vectors.
type Query struct {
	Text string
	Ref  *ArtifactRef
}

// StructuralFilters are filters on search-document facets (the HTTP route's filter parameters).
type StructuralFilters struct {
	CategoryPath      string
	TopicType         string
	SceneType         string
	ProvisionType     string
	ProductType       string
	RelationType      string
	InventoryCategory string
	Manufacturer      string
	Brand             string
	ModelNumber       string
	PartNumber        string
	ValidationStatus  string
}

// Filters apply to both candidate lists before fusion, so excluded artifacts take no result slot.
type Filters struct {
	ArtifactTypes      []string // empty = every artifact type
	RecordIDs          []int64  // only these records
	ExcludeRecordIDs   []int64
	ExcludeArtifactIDs []string
	KnowledgeStoreID   string
	DocumentGroup      string
	Structural         StructuralFilters
}

// Keyword is a caller keyword. Weight (default 1) changes the keyword list's ranking only;
// Required and Excluded remove results from the final list whichever list found them.
type Keyword struct {
	Term     string
	Weight   float64
	Required bool
	Excluded bool
}

// Criteria are the caller's conditions. Excluded keywords never enter the keyword list.
type Criteria struct {
	Keywords   []Keyword
	MinMatched int     // at least this many of the non-excluded keywords; 0 = no condition
	MinCosine  float64 // 0 = no floor
}

// KeywordExtraction selects the query-keyword LLM step for one request.
type KeywordExtraction string

const (
	KeywordExtractionDefault KeywordExtraction = ""    // the SEARCH_QUERY_KEYWORDS switch decides
	KeywordExtractionOff     KeywordExtraction = "off" // never extract for this request
	KeywordExtractionOn      KeywordExtraction = "on"  // extract (still needs a registered extractor)
)

type Options struct {
	Limit             int // results returned; 0 = DefaultLimit
	Offset            int // results skipped before Limit (paging over the fused list)
	KeywordExtraction KeywordExtraction
	// Trace asks for the candidate lists of the search in Response.Trace (benchmarks).
	Trace bool
}

type Request struct {
	Query    Query
	Filters  Filters
	Criteria Criteria
	Options  Options
}

// Diagnostics explain one result's score. A rank is absent when that list did not find it.
type Diagnostics struct {
	LexicalRank     *int     `json:"lexical_rank,omitempty"`
	LexicalScore    *float64 `json:"lexical_score,omitempty"` // BM25 (ParadeDB) or ts_rank_cd (Postgres): comparable within one backend and partition only
	VectorRank      *int     `json:"vector_rank,omitempty"`
	Cosine          *float64 `json:"cosine,omitempty"` // absent when the request has no query vector
	MatchedKeywords []string `json:"matched_keywords,omitempty"`
}

// Result is one artifact. Score is the fused score, wl/(60+lexical_rank)+ws/(60+vector_rank), a
// missing rank adding 0: it orders one response only, is not comparable across queries and is not a
// relevance probability. For an absolute question use Criteria.MinCosine or Diag.Cosine.
type Result struct {
	ArtifactType    string
	ArtifactID      string
	InputRecordID   int64
	Rank            int // 1-based position in the fused list (Options.Offset+1 for the first result)
	Score           float64
	Diag            Diagnostics
	PrimaryLabel    string
	SecondaryLabel  string
	Snippet         string
	SourceTitle     string
	SourceFilename  string
	SourceLineSpans json.RawMessage
	SemanticPayload json.RawMessage
	Keywords        []string
}

// Removed counts the fused candidates each criterion removed.
type Removed struct {
	Required   int `json:"required"`
	Excluded   int `json:"excluded"`
	MinMatched int `json:"min_matched"`
	MinCosine  int `json:"min_cosine"`
}

// TraceHit is one entry of a candidate list.
type TraceHit struct {
	ArtifactType string
	ArtifactID   string
	Score        float64 // BM25 / ts_rank_cd for the keyword lists; cosine similarity for the vector list
}

// TraceFused is one fused candidate before criteria and paging.
type TraceFused struct {
	ArtifactType string
	ArtifactID   string
	Score        float64
	LexicalRank  *int
	VectorRank   *int
}

// Trace holds the candidate lists a search built, in rank order.
type Trace struct {
	// LexicalPartitions are each partition's own list before merging (ParadeDB only), unsorted
	// across partitions.
	LexicalPartitions []TraceHit
	Lexical           []TraceHit // the merged keyword list the fusion used
	Vector            []TraceHit
	Fused             []TraceFused
}

type Response struct {
	Results    []Result
	Candidates int // fused candidates before criteria and the limit
	Removed    Removed
	// Keywords are the terms the keyword list used; empty when it used the raw query text.
	Keywords []string
	Trace    *Trace // set when Options.Trace is on
}

const (
	DefaultLimit   = 20
	maxLimit       = 200 // raised to the candidate limit when that is larger
	rrfK           = 60
	candidateLimit = 200
)

var (
	ErrNoQuery        = errors.New("searchengine: a query needs a text or a reference")
	ErrBothQueryForms = errors.New("searchengine: a query takes a text or a reference, not both")
	// ErrNoQueryVector is returned when Criteria.MinCosine is set but the request has no query vector
	// (semantic search off, no embedding model, or the embedding call failed).
	ErrNoQueryVector = errors.New("searchengine: a minimum cosine needs a query vector, and none is available (semantic search off, or no embedding model)")
	// ErrReferenceNotIndexed is wrapped by the error for a reference with no kb.search_artifacts row.
	ErrReferenceNotIndexed = errors.New("searchengine: reference is not in kb.search_artifacts")
)

func (r Request) validate() error {
	hasText := strings.TrimSpace(r.Query.Text) != ""
	hasRef := r.Query.Ref != nil
	switch {
	case hasText && hasRef:
		return ErrBothQueryForms
	case !hasText && !hasRef:
		return ErrNoQuery
	case hasRef && (strings.TrimSpace(r.Query.Ref.Type) == "" || strings.TrimSpace(r.Query.Ref.ID) == ""):
		return errors.New("searchengine: a reference needs an artifact type and id")
	}
	return nil
}

func (c Criteria) weight(k Keyword) float64 {
	if k.Weight > 0 {
		return k.Weight
	}
	return 1
}

// listKeywords are the criteria keywords that enter the keyword list.
func (c Criteria) listKeywords() []Keyword {
	var out []Keyword
	for _, k := range c.Keywords {
		if !k.Excluded && strings.TrimSpace(k.Term) != "" {
			out = append(out, Keyword{Term: strings.TrimSpace(k.Term), Weight: c.weight(k), Required: k.Required})
		}
	}
	return out
}
