package kbhandler

import "testing"

func TestParseArtifactSearchID(t *testing.T) {
	t.Run("summary", func(t *testing.T) {
		recordID, tail, err := parseArtifactSearchID("42_sum_1", "summary")
		if err != nil {
			t.Fatalf("parseArtifactSearchID(summary) err = %v", err)
		}
		if recordID != 42 || tail != "1" {
			t.Fatalf("parseArtifactSearchID(summary) = (%d, %q), want (42, %q)", recordID, tail, "1")
		}
	})

	t.Run("topic", func(t *testing.T) {
		recordID, tail, err := parseArtifactSearchID("9_tpc_3", "topic")
		if err != nil {
			t.Fatalf("parseArtifactSearchID(topic) err = %v", err)
		}
		if recordID != 9 || tail != "3" {
			t.Fatalf("parseArtifactSearchID(topic) = (%d, %q), want (9, %q)", recordID, tail, "3")
		}
	})

	t.Run("provision duplicated tail", func(t *testing.T) {
		recordID, tail, err := parseArtifactSearchID("203_prv_203_prv_5", "provision")
		if err != nil {
			t.Fatalf("parseArtifactSearchID(provision) err = %v", err)
		}
		if recordID != 203 || tail != "203_prv_5" {
			t.Fatalf("parseArtifactSearchID(provision) = (%d, %q), want (203, %q)", recordID, tail, "203_prv_5")
		}
	})
}

func TestParseArtifactSearchIDRejectsWrongTypeCode(t *testing.T) {
	if _, _, err := parseArtifactSearchID("42_tpc_1", "summary"); err == nil {
		t.Fatalf("parseArtifactSearchID accepted mismatched type code")
	}
}

func TestArtifactWikiProviderRegistryIncludesAdditionalSearchableTypes(t *testing.T) {
	for _, artifactType := range []string{"entity", "relation", "knowledge"} {
		if _, ok := getArtifactWikiProvider(artifactType); !ok {
			t.Fatalf("expected artifact wiki provider for %q", artifactType)
		}
	}
}
