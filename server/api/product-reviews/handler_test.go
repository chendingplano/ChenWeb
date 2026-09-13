package productreviews

import "testing"

// Scenario: profile list endpoint's limit clamping/parsing (spec:
// product-review-history-list) — absent/invalid/non-positive values fall
// back to the default, and oversized values clamp to max.
func TestParseListLimit(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"absent", "", 50},
		{"invalid", "abc", 50},
		{"zero", "0", 50},
		{"negative", "-5", 50},
		{"within range", "10", 10},
		{"oversized clamps to max", "9999", 200},
		{"exactly max", "200", 200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseListLimit(c.raw, defaultProfileListLimit, maxProfileListLimit); got != c.want {
				t.Errorf("parseListLimit(%q) = %d, want %d", c.raw, got, c.want)
			}
		})
	}
}
