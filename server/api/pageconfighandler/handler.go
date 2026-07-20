package pageconfighandler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
)

// resolvedEntry is one visible, authorized entry returned to the frontend.
type resolvedEntry struct {
	EntryKey string          `json:"entry_key"`
	Content  json.RawMessage `json:"content"`
}

type pageConfigResponse struct {
	Status  bool            `json:"status"`
	PageKey string          `json:"page_key"`
	Lang    string          `json:"lang"`
	Entries []resolvedEntry `json:"entries"`
	Hidden  []string        `json:"hidden"`
}

// GetPageConfig resolves a page's configurable content for the current user and
// locale, using an OVERLAY model: the page owns its structure and hardcoded
// defaults, and config only overrides text, hides, or restricts items.
//
//   - `entries`: entries that have a config row and are enabled AND authorized,
//     each with content resolved for the requested language (falling back to
//     [languages].default). The frontend applies these as label/description
//     overrides.
//   - `hidden`: entry_keys that have a config row but are disabled, suspended,
//     or unauthorized for this user. The frontend hides exactly these.
//
// An entry_key that is absent from BOTH lists has no config row, so the
// frontend renders the page's own hardcoded default (this is what a deleted
// entry falls back to). Unknown page keys log a diagnostic and return empty
// lists rather than an error.
// Endpoint: GET /api/v1/page-config/:pageKey?lang=<code>
func GetPageConfig(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_PGC_001")
	defer rc.Close()

	currentUser := rc.IsAuthenticated()
	if currentUser == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
			"loc":   "CWB_PGC_001",
		})
	}

	pageKey := strings.TrimSpace(c.Param("pageKey"))
	lang := normalizeLang(c.QueryParam("lang"))
	defaultLang := appconfig.GetDefaultLanguage()

	ctx := c.Request().Context()
	db := ApiTypes.ProjectDBHandle

	exists, err := pageExists(ctx, db, pageKey)
	if err != nil {
		rc.GetLogger().Error("page-config: page_def lookup failed", "page_key", pageKey, "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load page config"})
	}
	if !exists {
		// Fail open on rendering, but surface the likely typo for operators.
		rc.GetLogger().Warn("page-config: unknown page_key requested", "page_key", pageKey)
		return c.JSON(http.StatusOK, pageConfigResponse{
			Status: true, PageKey: pageKey, Lang: lang, Entries: []resolvedEntry{}, Hidden: []string{},
		})
	}

	rows, err := loadPageConfigRows(ctx, db, pageKey)
	if err != nil {
		rc.GetLogger().Error("page-config: load rows failed", "page_key", pageKey, "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load page config"})
	}

	validRoles := appconfig.GetAccessRoles(nil)
	entries, hidden := resolveEntries(groupByEntry(rows), currentUser, validRoles, lang, defaultLang)

	return c.JSON(http.StatusOK, pageConfigResponse{
		Status: true, PageKey: pageKey, Lang: lang, Entries: entries, Hidden: hidden,
	})
}

// resolveEntries applies access evaluation and locale resolution to the grouped
// rows (overlay model). It returns the enabled+authorized entries with resolved
// content, plus the entry_keys that have a row but must be hidden (disabled,
// suspended, or unauthorized for this user). Entries with a row but no
// default-language row are malformed and skipped (the frontend then falls back
// to its hardcoded default).
func resolveEntries(grouped map[string]entryRows, user *ApiTypes.UserInfo, validRoles []string, lang, defaultLang string) ([]resolvedEntry, []string) {
	entries := []resolvedEntry{}
	hidden := []string{}
	for entryKey, e := range grouped {
		defaultRow, ok := e.byLang[defaultLang]
		if !ok {
			continue
		}
		if !isAuthorized(defaultRow, user.Roles, validRoles) {
			hidden = append(hidden, entryKey)
			continue
		}
		content := defaultRow.Content
		if lang != "" && lang != defaultLang {
			if langRow, ok := e.byLang[lang]; ok {
				content = langRow.Content
			}
		}
		if len(content) == 0 {
			content = json.RawMessage("{}")
		}
		entries = append(entries, resolvedEntry{EntryKey: entryKey, Content: content})
	}
	return entries, hidden
}

// normalizeLang trims and lower-cases a requested language code.
func normalizeLang(lang string) string {
	return strings.ToLower(strings.TrimSpace(lang))
}
