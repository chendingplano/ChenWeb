package assertions

import "testing"

func TestParseProvisionModalityProhibited(t *testing.T) {
	if got := parseProvisionModality("The device shall not exceed 60 dB"); got != "prohibited" {
		t.Fatalf("expected prohibited, got %s", got)
	}
}

func TestParseProvisionModalityRequired(t *testing.T) {
	if got := parseProvisionModality("The manufacturer shall verify display luminance"); got != "required" {
		t.Fatalf("expected required, got %s", got)
	}
}

func TestParseProvisionModalityPermitted(t *testing.T) {
	if got := parseProvisionModality("The operator may adjust brightness"); got != "permitted" {
		t.Fatalf("expected permitted, got %s", got)
	}
}

func TestParseProvisionModalityRecommended(t *testing.T) {
	if got := parseProvisionModality("The device should be calibrated annually"); got != "recommended" {
		t.Fatalf("expected recommended, got %s", got)
	}
}

func TestParseProvisionModalityUnparsed(t *testing.T) {
	if got := parseProvisionModality("General background on device history"); got != "unparsed" {
		t.Fatalf("expected unparsed, got %s", got)
	}
}

func TestProvisionAssertionKindMapping(t *testing.T) {
	cases := map[string]string{
		"prohibited":  "prohibited",
		"required":    "required",
		"permitted":   "permitted",
		"recommended": "permitted",
		"unparsed":    "unparsed",
	}
	for modality, want := range cases {
		if got := provisionAssertionKind(modality); got != want {
			t.Errorf("provisionAssertionKind(%s) = %s, want %s", modality, got, want)
		}
	}
}
