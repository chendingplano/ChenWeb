package docreviews

import (
	"slices"
	"testing"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

func TestLimitMatchesToLLM_PreservesOrderAndCapsSlice(t *testing.T) {
	got := limitMatchesToLLM([]int{1, 2, 3, 4, 5})
	want := []int{1, 2, 3}
	if !slices.Equal(got, want) {
		t.Fatalf("limitMatchesToLLM() = %v, want %v", got, want)
	}
}

func TestMaxMatchesToLLM_ConfigOverridesEnvAndDefault(t *testing.T) {
	old := appconfig.AppConfig.DocReviewer.MaxArtifactsPassedToLLM
	t.Cleanup(func() { appconfig.AppConfig.DocReviewer.MaxArtifactsPassedToLLM = old })

	t.Setenv("MAX_MATCHES_TO_LLM", "8")
	appconfig.AppConfig.DocReviewer.MaxArtifactsPassedToLLM = 5
	if got := maxMatchesToLLM(); got != 5 {
		t.Fatalf("config=5 env=8: maxMatchesToLLM() = %d, want 5", got)
	}

	// Unset (0) and negative config values fall back to the env var.
	appconfig.AppConfig.DocReviewer.MaxArtifactsPassedToLLM = 0
	if got := maxMatchesToLLM(); got != 8 {
		t.Fatalf("config=0 env=8: maxMatchesToLLM() = %d, want 8", got)
	}
	appconfig.AppConfig.DocReviewer.MaxArtifactsPassedToLLM = -1
	if got := maxMatchesToLLM(); got != 8 {
		t.Fatalf("config=-1 env=8: maxMatchesToLLM() = %d, want 8", got)
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
