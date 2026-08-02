package docprocessing

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

const (
	FacetMethodDeterministic = "deterministic"
	FacetMethodMetadata      = "metadata"
	FacetMethodClassifier    = "classifier"
)

// FacetObservation is one immutable applicability fact observation.
type FacetObservation struct {
	ID                  int64
	RecordID            int64
	Path                string
	Value               any
	State               semrules.FactState
	Method              string
	Confidence          *float64
	Evidence            map[string]any
	SourceFingerprint   string
	DecisionAttemptID   string
	InvocationID        string
	VocabularyReleaseID int64
	Malformed           bool
}

func (o FacetObservation) ReleaseIDString() string {
	return int64String(o.VocabularyReleaseID)
}

func int64String(value int64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

// BuildApplicabilityFactSet reduces immutable observations to semrules facts.
func BuildApplicabilityFactSet(observations []FacetObservation) semrules.FactSet {
	return ReduceFacetObservations(observations)
}

// ReduceFacetObservations applies the deterministic P5 facet reduction order:
// deterministic > metadata > classifier, using only the highest usable rank.
func ReduceFacetObservations(observations []FacetObservation) semrules.FactSet {
	byPath := make(map[string][]FacetObservation)
	for _, observation := range observations {
		if observation.Path == "" {
			continue
		}
		byPath[observation.Path] = append(byPath[observation.Path], observation)
	}

	facts := make(semrules.FactSet, len(byPath))
	for path, pathObservations := range byPath {
		ranked := highestRankObservations(pathObservations)
		facts[path] = reduceOnePath(path, ranked)
	}
	return facts
}

func highestRankObservations(observations []FacetObservation) []FacetObservation {
	bestRank := -1
	var out []FacetObservation
	for _, observation := range observations {
		rank := facetMethodRank(observation.Method)
		if rank < 0 {
			continue
		}
		switch {
		case rank > bestRank:
			bestRank = rank
			out = []FacetObservation{observation}
		case rank == bestRank:
			out = append(out, observation)
		}
	}
	return out
}

func reduceOnePath(path string, observations []FacetObservation) semrules.Fact {
	fact := semrules.Fact{Path: path, State: semrules.FactMissing}
	if len(observations) == 0 {
		return fact
	}

	for _, observation := range observations {
		if observation.Malformed || observation.State == semrules.FactInvalid {
			fact.State = semrules.FactInvalid
			fact.Method = observation.Method
			fact.ReleaseID = observation.ReleaseIDString()
			return fact
		}
	}

	values := make(map[string]any)
	var minConfidence *float64
	var method, evidenceRef, runID, releaseID string
	for _, observation := range observations {
		if observation.State != "" && observation.State != semrules.FactKnown {
			if observation.State == semrules.FactConflicting {
				fact.State = semrules.FactConflicting
			}
			continue
		}
		key := canonicalFactValueKey(observation.Value)
		values[key] = observation.Value
		minConfidence = minFloat64Ptr(minConfidence, observation.Confidence)
		if method == "" {
			method = observation.Method
			evidenceRef = observation.SourceFingerprint
			runID = observation.DecisionAttemptID
			releaseID = observation.ReleaseIDString()
		}
	}

	if len(values) == 0 {
		if fact.State == semrules.FactConflicting {
			return fact
		}
		return semrules.Fact{Path: path, State: semrules.FactMissing}
	}
	if len(values) > 1 {
		return semrules.Fact{
			Path:        path,
			State:       semrules.FactConflicting,
			Value:       sortedValueList(values),
			Confidence:  minConfidence,
			Method:      method,
			EvidenceRef: evidenceRef,
			RunID:       runID,
			ReleaseID:   releaseID,
		}
	}
	return semrules.Fact{
		Path:        path,
		State:       semrules.FactKnown,
		Value:       firstMapValue(values),
		Confidence:  minConfidence,
		Method:      method,
		EvidenceRef: evidenceRef,
		RunID:       runID,
		ReleaseID:   releaseID,
	}
}

func facetMethodRank(method string) int {
	switch method {
	case FacetMethodClassifier:
		return 1
	case FacetMethodMetadata:
		return 2
	case FacetMethodDeterministic:
		return 3
	default:
		return -1
	}
}

func minFloat64Ptr(current, next *float64) *float64 {
	if next == nil {
		return current
	}
	if current == nil || *next < *current {
		value := *next
		return &value
	}
	return current
}

func canonicalFactValueKey(value any) string {
	normalized := normalizeObservationValue(value)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Sprintf("%T:%v", value, value)
	}
	return string(encoded)
}

func normalizeObservationValue(value any) any {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
		return value
	}
	values := make([]string, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		item, ok := rv.Index(i).Interface().(string)
		if !ok {
			return value
		}
		values = append(values, item)
	}
	sort.Strings(values)
	return values
}

func sortedValueList(values map[string]any) []any {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]any, 0, len(keys))
	for _, key := range keys {
		out = append(out, values[key])
	}
	return out
}

func firstMapValue(values map[string]any) any {
	for _, value := range values {
		return value
	}
	return nil
}
