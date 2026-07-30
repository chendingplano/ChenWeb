package comparison

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// unitDef is a linear conversion into one canonical unit per quantity kind.
// This is a deliberately minimal placeholder — see doc.go — good enough for
// the pilot fixture's Time/Luminance/Dimensionless/Angle/Count units, none of
// which need an offset (unlike temperature). DR13's `quantity` module
// replaces this with QUDT-sourced quantity kinds, units, and conversions.
type unitDef struct {
	QuantityKind string
	ToCanonical  decimal.Decimal // multiply a raw value in this unit by this factor to get the canonical-unit value
}

var unitRegistry = map[string]unitDef{
	"ms": {QuantityKind: "Time", ToCanonical: decimal.NewFromInt(1)},
	"s":  {QuantityKind: "Time", ToCanonical: decimal.NewFromInt(1000)}, // canonical unit for Time is ms

	"cd/m2": {QuantityKind: "Luminance", ToCanonical: decimal.NewFromInt(1)},

	"ratio":   {QuantityKind: "Dimensionless", ToCanonical: decimal.NewFromInt(1)},
	"px_pair": {QuantityKind: "Dimensionless", ToCanonical: decimal.NewFromInt(1)},
	// "1" is the unit string real extract_metrics output used for a ratio
	// value on at least one document (e.g. "1000:1" contrast extracted with
	// metric_unit="1" instead of "ratio" -- confirmed against a real
	// extraction run, ADR 2026072901 P3 finding). Same quantity kind, same
	// canonical factor: this is an alias, not a distinct unit.
	"1": {QuantityKind: "Dimensionless", ToCanonical: decimal.NewFromInt(1)},

	"degree": {QuantityKind: "Angle", ToCanonical: decimal.NewFromInt(1)},

	"count": {QuantityKind: "Count", ToCanonical: decimal.NewFromInt(1)},
}

// toCanonical converts v, expressed in unit, into the canonical value for
// quantityKind. It errors if the unit is unknown or belongs to a different
// quantity kind than the caller expects — a mismatch here means metric
// definition resolution went wrong upstream, not something a verdict should
// paper over.
func toCanonical(quantityKind, unit string, v decimal.Decimal) (decimal.Decimal, error) {
	def, ok := unitRegistry[strings.ToLower(strings.TrimSpace(unit))]
	if !ok {
		return decimal.Decimal{}, fmt.Errorf("comparison: unknown unit %q", unit)
	}
	if def.QuantityKind != quantityKind {
		return decimal.Decimal{}, fmt.Errorf(
			"comparison: unit %q belongs to quantity kind %q, not %q", unit, def.QuantityKind, quantityKind)
	}
	return v.Mul(def.ToCanonical), nil
}
