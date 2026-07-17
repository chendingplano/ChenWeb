package sitehandler

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

var validWorkspaceLangCode = regexp.MustCompile(`^[a-zA-Z0-9-]{1,20}$`)

type rawWorkspaceContentLabels struct {
	Labels       map[string]string `toml:"labels"`
	Descriptions map[string]string `toml:"descriptions"`
}

// LoadWorkspaceContentOverrides reads the [labels] and [descriptions] tables
// from config/workspace-content/labels-<lang>.toml, resolved relative to the
// repo root (the same directory that holds config.toml). An empty,
// unrecognized (non locale-code-shaped), or missing-file lang all resolve to
// two empty maps — no override is a fail-open state, not an error. Malformed
// TOML in an existing file is returned as an error so it's visible in logs
// rather than silently ignored. Mirrors kbhandler.LoadKnowledgeMenuLabels
// (ADR 2026071602), applied to the /semos/workspace page (ADR 2026071701),
// with [descriptions] added alongside [labels] so app tile subtitles can
// also be translated/renamed per language.
func LoadWorkspaceContentOverrides(lang string) (labels map[string]string, descriptions map[string]string, err error) {
	if !validWorkspaceLangCode.MatchString(lang) {
		return map[string]string{}, map[string]string{}, nil
	}

	path := resolveWorkspaceContentLabelsPath(lang)
	body, readErr := os.ReadFile(path)
	if readErr != nil {
		return map[string]string{}, map[string]string{}, nil
	}

	var raw rawWorkspaceContentLabels
	if unmarshalErr := toml.Unmarshal(body, &raw); unmarshalErr != nil {
		return nil, nil, unmarshalErr
	}

	labels = raw.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	descriptions = raw.Descriptions
	if descriptions == nil {
		descriptions = map[string]string{}
	}
	return labels, descriptions, nil
}

// resolveWorkspaceContentLabelsPath finds
// config/workspace-content/labels-<lang>.toml relative to the repo root,
// located the same way resolveKnowledgeContentLabelsPath locates
// config/knowledge-content/labels-<lang>.toml (walk up from cwd looking for
// config.toml). The WORKSPACE_CONTENT_LABELS_DIR env var overrides the
// resolved repo root, mirroring KNOWLEDGE_CONTENT_LABELS_DIR's role (used by
// tests to avoid depending on the working directory).
func resolveWorkspaceContentLabelsPath(lang string) string {
	if v := strings.TrimSpace(os.Getenv("WORKSPACE_CONTENT_LABELS_DIR")); v != "" {
		return filepath.Join(v, "labels-"+lang+".toml")
	}

	cur, _ := os.Getwd()
	for range 6 {
		if _, err := os.Stat(filepath.Join(cur, "config.toml")); err == nil {
			return filepath.Join(cur, "config", "workspace-content", "labels-"+lang+".toml")
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return filepath.Join("config", "workspace-content", "labels-"+lang+".toml")
}
