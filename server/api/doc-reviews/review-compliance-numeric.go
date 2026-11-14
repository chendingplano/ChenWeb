package docreviews

import (
	"fmt"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/comparison"
	"github.com/shopspring/decimal"
)

type complianceQuantity struct {
	Operator       string `json:"operator"`
	Value          string `json:"value"`
	Lower          string `json:"lower,omitempty"`
	Upper          string `json:"upper,omitempty"`
	Unit           string `json:"unit"`
	LowerExclusive bool   `json:"lower_exclusive,omitempty"`
	UpperExclusive bool   `json:"upper_exclusive,omitempty"`
}
type complianceNumeric struct {
	Property        string             `json:"property"`
	QuantityKind    string             `json:"quantity_kind"`
	ConditionsMatch bool               `json:"conditions_match"`
	MethodsMatch    bool               `json:"methods_match"`
	BasisMatch      bool               `json:"basis_match"`
	Required        complianceQuantity `json:"required"`
	Documented      complianceQuantity `json:"documented"`
}

func complianceConstraint(q complianceQuantity, kind string) (comparison.Constraint, error) {
	q.Operator = strings.TrimSpace(q.Operator)
	c := comparison.Constraint{QuantityKind: kind, Unit: q.Unit, LowerExclusive: q.LowerExclusive, UpperExclusive: q.UpperExclusive}
	var err error
	switch q.Operator {
	case ">", ">=":
		c.Form = comparison.FormLowerBound
		c.LowerExclusive = q.Operator == ">"
	case "<", "<=":
		c.Form = comparison.FormUpperBound
		c.UpperExclusive = q.Operator == "<"
	case "=":
		c.Form = comparison.FormExactValue
	case "range":
		c.Form = comparison.FormRange
	default:
		return c, fmt.Errorf("unsupported operator %q", q.Operator)
	}
	if c.Form == comparison.FormRange {
		c.LowerValue, err = decimal.NewFromString(q.Lower)
		if err != nil {
			return c, err
		}
		c.UpperValue, err = decimal.NewFromString(q.Upper)
	} else {
		c.Value, err = decimal.NewFromString(q.Value)
	}
	return c, err
}

func checkComplianceNumeric(n complianceNumeric) (string, string) {
	if n.Property == "" || n.QuantityKind == "" || !n.ConditionsMatch || !n.MethodsMatch || !n.BasisMatch {
		return "insufficient_evidence", "Property, conditions, method or measurement basis are not established as comparable."
	}
	required, err := complianceConstraint(n.Required, n.QuantityKind)
	if err != nil {
		return "insufficient_evidence", err.Error()
	}
	documented, err := complianceConstraint(n.Documented, n.QuantityKind)
	if err != nil {
		return "insufficient_evidence", err.Error()
	}
	verdict, reason, err := comparison.Compare(documented, required)
	if err != nil {
		return "insufficient_evidence", err.Error()
	}
	switch verdict {
	case comparison.Identical, comparison.Equivalent, comparison.Stronger:
		return "supported", reason
	case comparison.Weaker, comparison.Conflict:
		return "contradicted", reason
	default:
		return "insufficient_evidence", reason
	}
}
