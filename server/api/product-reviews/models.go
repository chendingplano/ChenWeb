package productreviews

import "time"

// Node kinds. A profile has exactly one `product` root; `module`/`part` nest
// under it recursively; `aspect` nodes attach to the root (parent_node_id null).
const (
	KindProduct = "product"
	KindModule  = "module"
	KindPart    = "part"
	KindAspect  = "aspect"
)

// Node origin — how the node entered the tree.
const (
	OriginLLMProposed   = "llm_proposed"
	OriginGraphExpanded = "graph_expanded"
	OriginUserAdded     = "user_added"
)

// Node curation status.
const (
	StatusProposed = "proposed"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
)

// Node grounding state — which governed space, if any, the label resolved to.
const (
	GroundingObjectNode     = "object_node"
	GroundingKeywordConcept = "keyword_concept"
	GroundingOntologyTerm   = "ontology_term"
	GroundingUngrounded     = "ungrounded"
)

// Aspect match mode: "join" resolves via kb.products.relation_type; "lexical"
// falls back to text/vector matching on the aspect's own labels.
const (
	MatchModeJoin    = "join"
	MatchModeLexical = "lexical"
)

// Profile status.
const (
	ProfileDraft = "draft"
	ProfileReady = "ready"
)

// reconcileNeedsReview is the set of kb.object_nodes.reconcile_status values that
// mean the app must not pick a sense on the user's behalf (spec: grounding pass).
var reconcileNeedsReview = map[string]bool{"ambiguous": true, "pending_review": true}

// Profile is a versioned product scope profile (kb.product_profiles).
type Profile struct {
	ID                 int64     `json:"id"`
	TenantID           string    `json:"tenant_id"`
	Name               string    `json:"name"`
	NameCN             string    `json:"name_cn"`
	NameEN             string    `json:"name_en"`
	ProductDescription string    `json:"product_description"`
	Keywords           []string  `json:"keywords"`
	Notes              string    `json:"notes"`
	Version            int       `json:"version"`
	Status             string    `json:"status"`
	Truncated          bool      `json:"truncated"`
	TruncatedCount     int       `json:"truncated_count"`
	DrawingID          *int64    `json:"drawing_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// ProfileSummary is a profile plus its latest review request/run, if any
// (spec: product-review-history-list). LatestRequestID/LatestRunID are nil
// when the profile has never had a review started; LatestRunStatus/
// LatestRunFinishedAt are zero when the profile has a request but no run yet.
type ProfileSummary struct {
	Profile
	LatestRequestID     *int64     `json:"latest_request_id,omitempty"`
	LatestRunID         *int64     `json:"latest_run_id,omitempty"`
	LatestRunStatus     string     `json:"latest_run_status,omitempty"`
	LatestRunFinishedAt *time.Time `json:"latest_run_finished_at,omitempty"`
	LatestMetricCount   *int       `json:"latest_metric_count,omitempty"`
}

// SourceRef records why an expanded node is in the tree: the kb.semantic_assertions
// row or kb.products row that supplied the edge.
type SourceRef struct {
	Kind   string `json:"kind"`   // "assertion" | "product"
	ID     int64  `json:"id"`     // kb row id
	Detail string `json:"detail"` // predicate_term_id or relation_type
}

// ProfileNode is one scope node (kb.product_profile_nodes).
type ProfileNode struct {
	ID              int64       `json:"id"`
	ProfileID       int64       `json:"profile_id"`
	ParentNodeID    *int64      `json:"parent_node_id"`
	NodeKind        string      `json:"node_kind"`
	Label           string      `json:"label"`
	LabelEN         string      `json:"label_en"`
	Aliases         []string    `json:"aliases"`
	Depth           int         `json:"depth"`
	Origin          string      `json:"origin"`
	Status          string      `json:"status"`
	Confidence      float64     `json:"confidence"`
	Rationale       string      `json:"rationale"`
	ObjectID        string      `json:"object_id"`
	ConceptID       string      `json:"concept_id"`
	TermID          string      `json:"term_id"`
	Grounding       string      `json:"grounding"`
	ReconcileStatus string      `json:"reconcile_status"`
	AspectKey       string      `json:"aspect_key"`
	RelationTypes   []string    `json:"relation_types"`
	MatchMode       string      `json:"match_mode"`
	SourceRefs      []SourceRef `json:"source_refs"`
}

// NeedsReconcileReview reports whether this node's grounded object is in an
// ambiguous / pending-review reconcile state and must be flagged to a reviewer.
func (n ProfileNode) NeedsReconcileReview() bool {
	return reconcileNeedsReview[n.ReconcileStatus]
}

// ProductNameEntry is one row of the kb.product_names catalog, trimmed to
// the fields the intake page's client-side name typeahead needs (design:
// product-name-typeahead — the catalog is a few thousand rows and changes
// rarely, so the browser loads it once and searches in memory rather than
// querying per keystroke).
type ProductNameEntry struct {
	ID            int64    `json:"id"`
	ProductName   string   `json:"product_name"`
	ProductNameEN string   `json:"product_name_en"`
	Aliases       []string `json:"aliases"`
	CategoryL1    string   `json:"category_l1"`
	CategoryL2    string   `json:"category_l2"`
	Status        string   `json:"status"`
}

// NewProfileInput is the payload for creating a profile.
type NewProfileInput struct {
	TenantID           string   `json:"tenant_id"`
	Name               string   `json:"name"`
	ProductNameEN      string   `json:"product_name_en"`
	ProductDescription string   `json:"product_description"`
	Keywords           []string `json:"keywords"`
	Notes              string   `json:"notes"`
}

// NodeEdit carries a mutable subset of a node's fields for an edit.
type NodeEdit struct {
	Label        *string
	LabelEN      *string
	Aliases      *[]string
	NodeKind     *string
	Status       *string
	ParentNodeID *int64 // set to change the parent; nil leaves it unchanged
}
