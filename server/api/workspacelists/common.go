// Package workspacelists implements the read and admin-CRUD APIs backing the
// three /semos/workspace status lists (announcements, recent activities,
// alarms/errors). See openspec/changes/workspace-lists-live-data for the
// proposal/design this package implements.
package workspacelists

import (
	"slices"
	"strings"
	"time"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// normalizeLang trims/lowercases a requested locale and falls back to the
// first configured language ([languages].languages in config.local.toml)
// when empty or not configured.
func normalizeLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	configured := appconfig.GetLanguages()
	if slices.Contains(configured, lang) {
		return lang
	}
	if len(configured) > 0 {
		return configured[0]
	}
	return "en"
}

// actorEmail returns the authenticated user's email, or "" when unavailable.
func actorEmail(rc ApiTypes.RequestContext) string {
	userInfo := rc.IsAuthenticated()
	if userInfo == nil {
		return ""
	}
	return strings.TrimSpace(userInfo.Email)
}

// parseOccurredAt parses an RFC3339 timestamp, defaulting to now (UTC) when
// raw is empty. The returned string is non-empty only when raw was non-empty
// and failed to parse.
func parseOccurredAt(raw string) (time.Time, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now().UTC(), ""
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, "occurred_at must be an RFC3339 timestamp"
	}
	return t.UTC(), ""
}

// requireAllLanguages trims translations in place and validates a non-empty
// entry exists for every configured language. The returned string is
// non-empty only on validation failure.
func requireAllLanguages(translations map[string]string) string {
	if translations == nil {
		return "translations is required"
	}
	for _, lang := range appconfig.GetLanguages() {
		text := strings.TrimSpace(translations[lang])
		if text == "" {
			return "translations." + lang + " is required"
		}
		translations[lang] = text
	}
	return ""
}
