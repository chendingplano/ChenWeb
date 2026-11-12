package complianceeval

import (
	"strings"
	"testing"
)

func TestParseGoldConfigDocumentsAndContext(t *testing.T) {
	raw := `
[compliance-review.covered-docs]
416 = "waste sorting standard"
371 = "green building"
[compliance-review.uncovered-docs]
799 = "AI agent paper"
[compliance-review.organization.operating_territories]
value = ["CN"]
status = "asserted"
source = "user"
`
	c, err := ParseGoldConfig([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	docs, _ := c.Documents()
	if len(docs) != 3 || docs[0].RecordID != 371 || !docs[0].Covered || docs[2].RecordID != 799 || docs[2].Covered {
		t.Fatalf("docs = %+v", docs)
	}
	ctx := c.OrganizationContext()
	if len(ctx) != len(OrgFactNames) || ctx["operating_territories"].Status != "asserted" || ctx["sectors"].Status != "unknown" {
		t.Fatalf("context = %+v", ctx)
	}
}

func TestParseGoldConfigRefusals(t *testing.T) {
	for name, tc := range map[string]struct{ raw, want string }{
		"both tables":    {"[compliance-review.covered-docs]\n416='a'\n[compliance-review.uncovered-docs]\n416='b'\n", "record 416 is listed in both"},
		"not an id":      {"[compliance-review.covered-docs]\nabc='a'\n", `"abc" is not a record id`},
		"unknown fact":   {"[compliance-review.organization.colour]\nstatus='asserted'\n", `unknown fact "colour"`},
		"bad status":     {"[compliance-review.organization.sectors]\nstatus='maybe'\n", `status "maybe"`},
		"missing status": {"[compliance-review.organization.sectors]\nvalue=['x']\n", `status ""`},
	} {
		if _, err := ParseGoldConfig([]byte(tc.raw)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", name, err, tc.want)
		}
	}
}

func TestParseGoldConfigAbsentTablesAllUnknown(t *testing.T) {
	c, err := ParseGoldConfig([]byte("[other]\nx=1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if docs, _ := c.Documents(); len(docs) != 0 {
		t.Fatalf("docs = %v", docs)
	}
	for n, f := range c.OrganizationContext() {
		if f.Status != "unknown" {
			t.Errorf("%s status = %s", n, f.Status)
		}
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
