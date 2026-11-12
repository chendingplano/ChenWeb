package complianceeval

import (
	"fmt"
	"strings"
)

// Environment variables naming the model profiles that label and adjudicate the gold set. Values
// are profile references resolved through MODEL_DEF_FILE; no model name lives in code.
const (
	EnvAnnotator1  = "COMPLIANCE_REVIEWER_ANNOTATOR_1"
	EnvAnnotator2  = "COMPLIANCE_REVIEWER_ANNOTATOR_2"
	EnvAdjudicator = "COMPLIANCE_ADJUDICATOR"
)

// Profiles are the three labelling model profiles.
type Profiles struct {
	Annotator1, Annotator2, Adjudicator string
}

// ResolveProfiles reads the three variables. An unset one is an error naming it, and the two
// annotators must differ so their labels are independent.
func ResolveProfiles(getenv func(string) string) (Profiles, error) {
	get := func(name string) (string, error) {
		v := strings.TrimSpace(getenv(name))
		if v == "" {
			return "", fmt.Errorf("%s is not set: set it to a model profile defined in MODEL_DEF_FILE", name)
		}
		return v, nil
	}
	var p Profiles
	var err error
	if p.Annotator1, err = get(EnvAnnotator1); err != nil {
		return Profiles{}, err
	}
	if p.Annotator2, err = get(EnvAnnotator2); err != nil {
		return Profiles{}, err
	}
	if p.Adjudicator, err = get(EnvAdjudicator); err != nil {
		return Profiles{}, err
	}
	if p.Annotator1 == p.Annotator2 {
		return Profiles{}, fmt.Errorf("%s and %s name the same profile %q; the annotators must differ", EnvAnnotator1, EnvAnnotator2, p.Annotator1)
	}
	return p, nil
}
