package comparison

import (
	"encoding/json"
	"errors"
	"strings"
)

// DirectionalCellInput contains the already-selected subject constraint and
// one authority family's evidence. Representative selection is deliberately
// outside this function so scope policy remains explicit and auditable.
type DirectionalCellInput struct {
	RunID                              int64
	TargetObjectID                     string
	MetricKey                          string
	SubjectFamily                      string
	SubjectConstraint                  Constraint
	SubjectRepresentativeAssertionID   int64
	SubjectAssertions                  json.RawMessage
	SubjectRemainderCount              int64
	AuthorityFamily                    string
	AuthorityEvidence                  FamilyEvidence
	AuthorityRepresentativeAssertionID int64
	AuthorityAssertions                json.RawMessage
	AuthorityRemainderCount            int64
}

// EvaluateDirectionalCell derives a persistable DR22 cell using the existing
// DR21 comparator. Verdict direction is invariant: the subject side is
// always compared to the authority family, never inferred from table layout.
func EvaluateDirectionalCell(in DirectionalCellInput) (ComparisonCell, error) {
	if in.RunID == 0 || strings.TrimSpace(in.TargetObjectID) == "" || strings.TrimSpace(in.MetricKey) == "" || strings.TrimSpace(in.SubjectFamily) == "" || strings.TrimSpace(in.AuthorityFamily) == "" {
		return ComparisonCell{}, errors.New("directional cell identity is required")
	}
	verdict, rationale, err := EvaluateFamily(in.SubjectConstraint, in.AuthorityEvidence)
	if err != nil {
		return ComparisonCell{}, err
	}
	return ComparisonCell{
		RunID:                              in.RunID,
		TargetObjectID:                     in.TargetObjectID,
		MetricKey:                          in.MetricKey,
		SubjectFamily:                      in.SubjectFamily,
		SubjectRepresentativeAssertionID:   in.SubjectRepresentativeAssertionID,
		SubjectAssertions:                  in.SubjectAssertions,
		SubjectRemainderCount:              in.SubjectRemainderCount,
		AuthorityFamily:                    in.AuthorityFamily,
		AuthorityRepresentativeAssertionID: in.AuthorityRepresentativeAssertionID,
		AuthorityAssertions:                in.AuthorityAssertions,
		AuthorityRemainderCount:            in.AuthorityRemainderCount,
		Verdict:                            verdict,
		Direction:                          "subject_to_authority",
		Rationale:                          rationale,
	}, nil
}
