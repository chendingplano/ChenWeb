package docreviews

import (
	"slices"
	"testing"
)

func TestLimitMatchesToLLM_PreservesOrderAndCapsSlice(t *testing.T) {
	got := limitMatchesToLLM([]int{1, 2, 3, 4, 5})
	want := []int{1, 2, 3}
	if !slices.Equal(got, want) {
		t.Fatalf("limitMatchesToLLM() = %v, want %v", got, want)
	}
}

func TestMaxMatchesToLLM_DefaultsAndMinimum(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  int
	}{
		{name: "unset", value: "", want: 3},
		{name: "blank", value: "  ", want: 3},
		{name: "malformed", value: "three", want: 3},
		{name: "overflow", value: "999999999999999999999999999999", want: 3},
		{name: "zero", value: "0", want: 1},
		{name: "negative", value: "-7", want: 1},
		{name: "positive", value: "8", want: 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MAX_MATCHES_TO_LLM", tc.value)
			if got := maxMatchesToLLM(); got != tc.want {
				t.Fatalf("MAX_MATCHES_TO_LLM=%q: got %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}
