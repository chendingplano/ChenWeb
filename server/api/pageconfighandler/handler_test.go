package pageconfighandler

import (
	"encoding/json"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

var testValidRoles = []string{"admin", "root", "guest", "dev", "k_engineer", "trial"}

func row(accessRole []string, accessible, enabled bool, content string) configRow {
	return configRow{
		AccessRole: accessRole,
		Accessible: accessible,
		Enabled:    enabled,
		Content:    json.RawMessage(content),
	}
}

func TestIsAuthorized(t *testing.T) {
	cases := []struct {
		name       string
		accessRole []string
		accessible bool
		enabled    bool
		userRoles  []string
		want       bool
	}{
		{"enabled accessible matching role", []string{"admin"}, true, true, []string{"admin"}, true},
		{"disabled hides from everyone", []string{"admin"}, true, false, []string{"admin"}, false},
		{"accessible flag off hides from everyone", []string{"admin"}, false, true, []string{"admin"}, false},
		{"empty access_role suspends", []string{}, true, true, []string{"admin"}, false},
		{"nil access_role suspends", nil, true, true, []string{"admin"}, false},
		{"only invalid tokens suspends", []string{"nonsense"}, true, true, []string{"nonsense"}, false},
		{"valid role but user lacks it", []string{"admin"}, true, true, []string{"guest"}, false},
		{"user holds one of several roles", []string{"admin", "dev"}, true, true, []string{"dev"}, true},
		{"case-insensitive match", []string{"Admin"}, true, true, []string{"ADMIN"}, true},
		{"invalid token ignored, valid one honored", []string{"nonsense", "guest"}, true, true, []string{"guest"}, true},
		{"user with no roles", []string{"admin"}, true, true, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isAuthorized(row(tc.accessRole, tc.accessible, tc.enabled, "{}"), tc.userRoles, testValidRoles)
			if got != tc.want {
				t.Fatalf("isAuthorized(%+v, roles=%v) = %v, want %v", tc.accessRole, tc.userRoles, got, tc.want)
			}
		})
	}
}

func TestResolveEntriesOmitsUnauthorizedAndDisabled(t *testing.T) {
	user := &ApiTypes.UserInfo{Roles: []string{"guest"}}
	grouped := map[string]entryRows{
		"visible":    {byLang: map[string]configRow{"en": row([]string{"guest"}, true, true, `{"label":"V"}`)}},
		"disabled":   {byLang: map[string]configRow{"en": row([]string{"guest"}, true, false, `{"label":"D"}`)}},
		"suspended":  {byLang: map[string]configRow{"en": row(nil, true, true, `{"label":"S"}`)}},
		"unauth":     {byLang: map[string]configRow{"en": row([]string{"admin"}, true, true, `{"label":"U"}`)}},
		"no-default": {byLang: map[string]configRow{"zh-cn": row([]string{"guest"}, true, true, `{"label":"N"}`)}},
	}

	entries, hidden := resolveEntries(grouped, user, testValidRoles, "en", "en")

	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 visible entry, got %d: %+v", len(entries), entries)
	}
	if entries[0].EntryKey != "visible" {
		t.Fatalf("expected 'visible' entry, got %q", entries[0].EntryKey)
	}
	// disabled + suspended + unauth have rows -> hidden; no-default is skipped.
	hiddenSet := map[string]bool{}
	for _, k := range hidden {
		hiddenSet[k] = true
	}
	for _, k := range []string{"disabled", "suspended", "unauth"} {
		if !hiddenSet[k] {
			t.Fatalf("expected %q in hidden list, got %v", k, hidden)
		}
	}
	if hiddenSet["no-default"] {
		t.Fatalf("entry with no default-language row should be skipped, not hidden")
	}
}

func TestResolveEntriesLocaleFallback(t *testing.T) {
	user := &ApiTypes.UserInfo{Roles: []string{"guest"}}
	// default lang = zh-cn (authoritative for access); en is the requested lang.
	grouped := map[string]entryRows{
		"has-en": {byLang: map[string]configRow{
			"zh-cn": row([]string{"guest"}, true, true, `{"label":"中文"}`),
			"en":    row([]string{"guest"}, true, true, `{"label":"English"}`),
		}},
		"zh-only": {byLang: map[string]configRow{
			"zh-cn": row([]string{"guest"}, true, true, `{"label":"仅中文"}`),
		}},
	}

	entries, _ := resolveEntries(grouped, user, testValidRoles, "en", "zh-cn")

	byKey := map[string]string{}
	for _, e := range entries {
		var content struct {
			Label string `json:"label"`
		}
		_ = json.Unmarshal(e.Content, &content)
		byKey[e.EntryKey] = content.Label
	}
	if byKey["has-en"] != "English" {
		t.Fatalf("requested-lang (en) content expected 'English', got %q", byKey["has-en"])
	}
	if byKey["zh-only"] != "仅中文" {
		t.Fatalf("missing en row should fall back to default zh-cn content '仅中文', got %q", byKey["zh-only"])
	}
}

func TestNormalizeRoles(t *testing.T) {
	got := normalizeRoles([]string{" Admin ", "admin", "", "DEV"})
	want := []string{"admin", "dev"}
	if len(got) != len(want) {
		t.Fatalf("normalizeRoles = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeRoles = %v, want %v", got, want)
		}
	}
}
