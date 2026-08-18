package classfoundation

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ObservedProfileObservation is one source-backed structural observation. Its
// state is intentionally required: malformed, missing, unparsed, conflicting,
// and conforming observations are all retained as independent evidence.
type ObservedProfileObservation struct {
	ClassTermID       string
	AttributeKey      string
	ObservedName      string
	LogicalDatatype   string
	ValueForm         string
	UnitTermID        string
	Cardinality       string
	ObservationState  string
	AggregationMethod string
	MethodVersion     string
	DocumentKey       string
	AssertionID       *int64
	EvidenceID        *int64
	RawValue          string
	NormalizedValue   string
	SourceExcerpt     string
	ExceptionKind     string
	ExceptionDetails  string
	Confidence        *float64
	By                string
}

// ObservedProfileStore records evidence-only class aggregation. It neither
// reads nor writes class-contract tables, so aggregation cannot grant contract
// authority or discard a problematic source observation.
type ObservedProfileStore struct {
	DB DBX
}

func (s ObservedProfileStore) Record(ctx context.Context, observation ObservedProfileObservation) error {
	if err := validateObservedProfileObservation(observation); err != nil {
		return err
	}
	if s.DB == nil {
		return errors.New("db is nil")
	}
	profileID, err := s.profileID(ctx, observation)
	if err != nil {
		return err
	}
	attributeID, err := s.attributeID(ctx, profileID, observation)
	if err != nil {
		return err
	}
	if err := s.recordDistribution(ctx, attributeID, observation); err != nil {
		return err
	}
	if err := s.recordExample(ctx, profileID, attributeID, observation); err != nil {
		return err
	}
	if err := s.recordException(ctx, profileID, attributeID, observation); err != nil {
		return err
	}
	return nil
}

func (s ObservedProfileStore) profileID(ctx context.Context, observation ObservedProfileObservation) (int64, error) {
	var profileID int64
	err := s.DB.QueryRowContext(ctx, `
INSERT INTO kb.ontology_observed_class_profiles (
    class_term_id, aggregation_method, method_version, create_by, modify_by
)
VALUES ($1, $2, $3, $4, $4)
ON CONFLICT (class_term_id) DO UPDATE
SET aggregation_method = EXCLUDED.aggregation_method,
    method_version = EXCLUDED.method_version,
    modify_time = NOW(),
    modify_by = EXCLUDED.modify_by
RETURNING id`, strings.TrimSpace(observation.ClassTermID), strings.TrimSpace(observation.AggregationMethod), strings.TrimSpace(observation.MethodVersion), nullable(observation.By)).Scan(&profileID)
	if err != nil {
		return 0, fmt.Errorf("upsert observed profile: %w", err)
	}
	return profileID, nil
}

func (s ObservedProfileStore) attributeID(ctx context.Context, profileID int64, observation ObservedProfileObservation) (int64, error) {
	var attributeID int64
	err := s.DB.QueryRowContext(ctx, `
INSERT INTO kb.ontology_observed_class_attribute_observations (
    profile_id, attribute_key, observed_name, logical_datatype, value_form,
    unit_term_id, cardinality_observation, observation_state, grouping_method,
    confidence, observed_count, document_count
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 1, 1)
ON CONFLICT (profile_id, attribute_key, logical_datatype, value_form, unit_term_id, observation_state)
DO UPDATE SET observed_count = kb.ontology_observed_class_attribute_observations.observed_count + 1,
              document_count = kb.ontology_observed_class_attribute_observations.document_count + 1,
              modify_time = NOW()
RETURNING id`, profileID, strings.TrimSpace(observation.AttributeKey), nullable(observation.ObservedName), strings.TrimSpace(observation.LogicalDatatype), strings.TrimSpace(observation.ValueForm), strings.TrimSpace(observation.UnitTermID), nullable(observation.Cardinality), strings.TrimSpace(observation.ObservationState), strings.TrimSpace(observation.AggregationMethod), observation.Confidence).Scan(&attributeID)
	if err != nil {
		return 0, fmt.Errorf("upsert observed attribute: %w", err)
	}
	return attributeID, nil
}

func (s ObservedProfileStore) recordDistribution(ctx context.Context, attributeID int64, observation ObservedProfileObservation) error {
	if strings.TrimSpace(observation.DocumentKey) == "" {
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO kb.ontology_observed_class_attribute_distributions (
    attribute_observation_id, distribution_kind, distribution_value,
    observed_count, document_count, evidence_summary
)
VALUES ($1, $2, $3, 1, 1, $4::jsonb)
ON CONFLICT (attribute_observation_id, distribution_kind, distribution_value)
DO UPDATE SET observed_count = kb.ontology_observed_class_attribute_distributions.observed_count + 1,
              document_count = kb.ontology_observed_class_attribute_distributions.document_count + 1,
              modify_time = NOW()`, attributeID, "document", strings.TrimSpace(observation.DocumentKey), "{}")
	if err != nil {
		return fmt.Errorf("record observed distribution: %w", err)
	}
	return nil
}

func (s ObservedProfileStore) recordExample(ctx context.Context, profileID, attributeID int64, observation ObservedProfileObservation) error {
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO kb.ontology_observed_class_profile_examples (
    profile_id, attribute_observation_id, assertion_id, evidence_id,
    observation_state, raw_value, normalized_value, source_excerpt, method, confidence
)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)`,
		profileID, attributeID, observation.AssertionID, observation.EvidenceID,
		strings.TrimSpace(observation.ObservationState), nullable(observation.RawValue), nullableJSON(observation.NormalizedValue), nullable(observation.SourceExcerpt), strings.TrimSpace(observation.AggregationMethod), observation.Confidence)
	if err != nil {
		return fmt.Errorf("record observed example: %w", err)
	}
	return nil
}

func (s ObservedProfileStore) recordException(ctx context.Context, profileID, attributeID int64, observation ObservedProfileObservation) error {
	if strings.TrimSpace(observation.ExceptionKind) == "" {
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO kb.ontology_observed_class_profile_exceptions (
    profile_id, attribute_observation_id, assertion_id, evidence_id,
    exception_kind, observation_state, details, method, confidence
)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9)`,
		profileID, attributeID, observation.AssertionID, observation.EvidenceID,
		strings.TrimSpace(observation.ExceptionKind), strings.TrimSpace(observation.ObservationState), normalizedJSON(observation.ExceptionDetails), strings.TrimSpace(observation.AggregationMethod), observation.Confidence)
	if err != nil {
		return fmt.Errorf("record observed exception: %w", err)
	}
	return nil
}

func validateObservedProfileObservation(observation ObservedProfileObservation) error {
	if strings.TrimSpace(observation.ClassTermID) == "" || strings.TrimSpace(observation.AttributeKey) == "" {
		return errors.New("class term ID and attribute key are required")
	}
	if strings.TrimSpace(observation.ObservationState) == "" {
		return errors.New("observation state is required; observations cannot be dropped")
	}
	if strings.TrimSpace(observation.AggregationMethod) == "" || strings.TrimSpace(observation.MethodVersion) == "" {
		return errors.New("aggregation method and version are required")
	}
	if kind := strings.TrimSpace(observation.ExceptionKind); kind != "" && kind != "outlier" && kind != "contradiction" {
		return fmt.Errorf("unsupported observed exception kind %q", kind)
	}
	return nil
}
