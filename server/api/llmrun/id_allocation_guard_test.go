package llmrun

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// idCounterAllowlist names files whose counter-like SQL is not an ad-hoc ID generator, with the
// reason. Anything else must take IDs from the shared ID manager (shared/go/api/idmanager).
var idCounterAllowlist = map[string]string{
	"project_migrations/20261024000015_add_search_run_id.sql":                          "historical: search run IDs, replaced by llm_run_id; applied migrations are never edited",
	"project_migrations/20260801000023_add_unique_version_to_kb_pipeline_policies.sql": "comment describing per-policy version numbers, not an ID",
	"server/api/calendarhandler/store.go":                                              "display_seqno is a per-calendar sort position, not an ID",
	"server/api/workspacelists/activities.go":                                          "reserves the inserted row's own BIGSERIAL primary key",
	"server/api/workspacelists/announcements.go":                                       "reserves the inserted row's own BIGSERIAL primary key",
}

// TestNoAdHocIDCounters keeps new ID generators out of ChenWeb: a run, batch, job or correlation
// ID, or any ID that is not the inserted row's own primary key, comes from the shared ID manager
// (see the root CLAUDE.md "Reusable modules" section). It fails on CREATE SEQUENCE, nextval( and
// MAX(x)+1 in non-test Go code and migrations outside idCounterAllowlist.
func TestNoAdHocIDCounters(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	counter := regexp.MustCompile(`(?i)create\s+sequence|nextval\s*\(|max\s*\(\s*[a-z_.]+\s*\)\s*\+\s*1`)
	var bad []string
	for _, dir := range []string{"server", "project_migrations", "shared_migrations"} {
		err := filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || strings.HasSuffix(path, "_test.go") ||
				!(strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".sql")) {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if _, ok := idCounterAllowlist[rel]; ok {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(raw), "\n") {
				if counter.MatchString(line) {
					bad = append(bad, rel+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
				}
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	for rel := range idCounterAllowlist {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("idCounterAllowlist names a missing file; remove it: %s", rel)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("ad-hoc ID counter found. Take IDs that are not the row's own primary key from "+
			"shared/go/api/idmanager (llmrun shows how). If this is the row's own primary key or not "+
			"an ID, add the file to idCounterAllowlist with the reason.\n%s", strings.Join(bad, "\n"))
	}
}
