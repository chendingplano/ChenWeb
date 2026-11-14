package complianceeval

import (
	"strings"
	"testing"
)

func TestParseGoldConfigDocuments(t *testing.T) {
	raw := `
[compliance-review.gold-docs]
416 = "waste sorting standard"
371 = "green building"
799 = "AI agent paper"
`
	c, err := ParseGoldConfig([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	docs, _ := c.Documents()
	if len(docs) != 3 || docs[0].RecordID != 371 || docs[0].Reason != "green building" || docs[2].RecordID != 799 {
		t.Fatalf("docs = %+v", docs)
	}
}

func TestParseGoldConfigRefusals(t *testing.T) {
	for name, tc := range map[string]struct{ raw, want string }{
		"retired covered table":   {"[compliance-review.covered-docs]\n416='a'\n", "merged into [compliance-review.gold-docs]"},
		"retired uncovered table": {"[compliance-review.uncovered-docs]\n", "merged into [compliance-review.gold-docs]"},
		"listed twice":            {"[compliance-review.gold-docs]\n416='a'\n0416='b'\n", "record 416 is listed twice"},
		"not an id":               {"[compliance-review.gold-docs]\nabc='a'\n", `"abc" is not a record id`},
	} {
		if _, err := ParseGoldConfig([]byte(tc.raw)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", name, err, tc.want)
		}
	}
}

func TestParseGoldConfigAbsentTables(t *testing.T) {
	c, err := ParseGoldConfig([]byte("[other]\nx=1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if docs, _ := c.Documents(); len(docs) != 0 {
		t.Fatalf("docs = %v", docs)
	}
}

func TestParseGoldConfigMaxCandidates(t *testing.T) {
	c, err := ParseGoldConfig([]byte("[compliance-review.gold-docs]\n416='a'\n"))
	if err != nil || c.MaxCandidates != DefaultMaxCandidates {
		t.Fatalf("absent: max=%v err=%v, want %d", c, err, DefaultMaxCandidates)
	}
	c, err = ParseGoldConfig([]byte("[reviewers.compliance]\nmax_candidates = 50\n"))
	if err != nil || c.MaxCandidates != 50 {
		t.Fatalf("set: c=%+v err=%v", c, err)
	}
	for _, bad := range []string{"0", "-3"} {
		if _, err := ParseGoldConfig([]byte("[reviewers.compliance]\nmax_candidates = " + bad + "\n")); err == nil || !strings.Contains(err.Error(), "max_candidates") {
			t.Errorf("max_candidates=%s: err = %v", bad, err)
		}
	}
	c, _ = ParseGoldConfig([]byte("[compliance-review.gold-docs]\n799='paper'\n"))
	if d, ok := c.Find(799); !ok || d.Reason != "paper" {
		t.Errorf("find 799 = %+v %v", d, ok)
	}
	if _, ok := c.Find(1); ok {
		t.Errorf("record 1 is not listed")
	}
}

func TestResolveProfiles(t *testing.T) {
	env := map[string]string{EnvAnnotator1: "a", EnvAnnotator2: "b", EnvAdjudicator: "c"}
	get := func(k string) string { return env[k] }
	if p, err := ResolveProfiles(get); err != nil || p != (Profiles{"a", "b", "c"}) {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	for _, name := range []string{EnvAnnotator1, EnvAnnotator2, EnvAdjudicator} {
		saved := env[name]
		env[name] = ""
		if _, err := ResolveProfiles(get); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("unset %s: err = %v", name, err)
		}
		env[name] = saved
	}
	env[EnvAnnotator2] = "a"
	if _, err := ResolveProfiles(get); err == nil || !strings.Contains(err.Error(), "must differ") {
		t.Errorf("same annotators: err = %v", err)
	}
}
