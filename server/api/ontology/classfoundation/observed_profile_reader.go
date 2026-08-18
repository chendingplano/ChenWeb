package classfoundation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const observedProfileAuthority = "observed evidence; non-authoritative"

// ObservedProfile is a bounded evidence read model. It is deliberately not a
// class contract and consumers must not treat it as an authority decision.
type ObservedProfile struct {
	ID                int64
	ClassTermID       string
	AggregationMethod string
	MethodVersion     string
	Confidence        *float64
	Authoritative     bool
	Authority         string
	Examples          []ObservedProfileExample
}

type ObservedProfileExample struct {
	ID               int64
	ObservationState string
	RawValue         string
	SourceExcerpt    string
	Method           string
	Confidence       *float64
	CreateTime       time.Time
}

// ObservedProfileReader reads profile evidence with a database-side example
// cap. It never joins or derives authoritative class-contract state.
type ObservedProfileReader struct {
	DB   DBX
	Caps Caps
}

func (r ObservedProfileReader) Get(ctx context.Context, classTermID string) (ObservedProfile, error) {
	if r.DB == nil {
		return ObservedProfile{}, errors.New("db is nil")
	}
	classTermID = strings.TrimSpace(classTermID)
	if classTermID == "" {
		return ObservedProfile{}, errors.New("class term ID is required")
	}
	var (
		profile    ObservedProfile
		confidence sql.NullFloat64
	)
	err := r.DB.QueryRowContext(ctx, `
SELECT p.id, p.class_term_id, p.aggregation_method, p.method_version, p.confidence
FROM kb.ontology_observed_class_profiles p
WHERE p.class_term_id = $1`, classTermID).Scan(&profile.ID, &profile.ClassTermID, &profile.AggregationMethod, &profile.MethodVersion, &confidence)
	if err != nil {
		return ObservedProfile{}, fmt.Errorf("load observed profile: %w", err)
	}
	if confidence.Valid {
		profile.Confidence = &confidence.Float64
	}
	profile.Authoritative = false
	profile.Authority = observedProfileAuthority

	cap := r.Caps.ProfileExamples
	if cap < 1 {
		cap = CapsFromEnv().ProfileExamples
	}
	rows, err := r.DB.QueryContext(ctx, `
SELECT id, observation_state, raw_value, source_excerpt, method, confidence, create_time
FROM kb.ontology_observed_class_profile_examples
WHERE profile_id = $1
ORDER BY create_time DESC, id DESC
LIMIT $2`, profile.ID, cap)
	if err != nil {
		return ObservedProfile{}, fmt.Errorf("load observed profile examples: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			example                 ObservedProfileExample
			rawValue, sourceExcerpt sql.NullString
			exampleConfidence       sql.NullFloat64
		)
		if err := rows.Scan(&example.ID, &example.ObservationState, &rawValue, &sourceExcerpt, &example.Method, &exampleConfidence, &example.CreateTime); err != nil {
			return ObservedProfile{}, fmt.Errorf("scan observed profile example: %w", err)
		}
		if rawValue.Valid {
			example.RawValue = rawValue.String
		}
		if sourceExcerpt.Valid {
			example.SourceExcerpt = sourceExcerpt.String
		}
		if exampleConfidence.Valid {
			example.Confidence = &exampleConfidence.Float64
		}
		profile.Examples = append(profile.Examples, example)
	}
	if err := rows.Err(); err != nil {
		return ObservedProfile{}, fmt.Errorf("iterate observed profile examples: %w", err)
	}
	return profile, nil
}
