package semrules

// Truth is the three-valued result of evaluating a predicate.
type Truth string

const (
	TruthTrue          Truth = "true"
	TruthFalse         Truth = "false"
	TruthIndeterminate Truth = "indeterminate"
)

// Document is a persisted, independently versioned predicate document.
type Document struct {
	Version    int       `json:"version"`
	Expression Predicate `json:"expression"`
}

// Predicate is a node in a predicate expression tree.
type Predicate struct {
	Kind          string      `json:"kind"`
	Path          string      `json:"path,omitempty"`
	Op            string      `json:"op,omitempty"`
	Value         any         `json:"value,omitempty"`
	MinConfidence *float64    `json:"min_confidence,omitempty"`
	Items         []Predicate `json:"items,omitempty"`

	// Facet is the legacy, unnamespaced fact key. New predicate documents use
	// Path. It remains until callers of Evaluate migrate to typed facts.
	Facet string `json:"facet,omitempty"`
}

// FactState describes whether a fact can participate in evaluation.
type FactState string

const (
	FactKnown       FactState = "known"
	FactMissing     FactState = "missing"
	FactConflicting FactState = "conflicting"
	FactInvalid     FactState = "invalid"
)

// FactType is the canonical runtime value type declared for a fact path.
// Governed term identifiers are strings with a GovernedValueScheme on their
// PathSpec, rather than a separate runtime value type.
type FactType string

const (
	FactTypeString    FactType = "string"
	FactTypeNumber    FactType = "number"
	FactTypeBoolean   FactType = "boolean"
	FactTypeDate      FactType = "date"
	FactTypeStringSet FactType = "string_set"
)

// KnownValue is a usable fact value paired with the type declared by its
// registered path. Typed operators receive only known values; fact-state
// handling occurs before operator dispatch.
type KnownValue struct {
	Type  FactType
	Value any
}

// Fact is a typed observation supplied to the predicate evaluator.
type Fact struct {
	Path        string    `json:"path"`
	Value       any       `json:"value,omitempty"`
	State       FactState `json:"state"`
	Confidence  *float64  `json:"confidence,omitempty"`
	Method      string    `json:"method,omitempty"`
	EvidenceRef string    `json:"evidence_ref,omitempty"`
	RunID       string    `json:"run_id,omitempty"`
	PolicyID    string    `json:"policy_id,omitempty"`
	ReleaseID   string    `json:"release_id,omitempty"`
}

// PathSpec is code-owned metadata for one canonical fact path.
type PathSpec struct {
	Path                string   `json:"path"`
	Namespace           string   `json:"namespace"`
	Type                FactType `json:"type"`
	Operators           []string `json:"operators"`
	Tier3Producible     bool     `json:"tier_3_producible"`
	GovernedValueScheme string   `json:"governed_value_scheme,omitempty"`
}
