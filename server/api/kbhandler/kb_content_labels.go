package kbhandler

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

var validLangCode = regexp.MustCompile(`^[a-zA-Z0-9-]{1,20}$`)

type rawKnowledgeContentLabels struct {
	Labels map[string]string `toml:"labels"`
}

// LoadKnowledgeContentLabels reads the [labels] table from
// config/knowledge-content/labels-<lang>.toml, resolved relative to the repo
// root (the same directory that holds config.toml). An empty, unrecognized
// (non locale-code-shaped), or missing-file lang all resolve to an empty
// map — no override is a fail-open state, not an error. Malformed TOML in
// an existing file is returned as an error so it's visible in logs rather
// than silently ignored.
func LoadKnowledgeContentLabels(lang string) (map[string]string, error) {
	if !validLangCode.MatchString(lang) {
		return map[string]string{}, nil
	}

	path := resolveKnowledgeContentLabelsPath(lang)
	body, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}, nil
	}

	var raw rawKnowledgeContentLabels
	if err := toml.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if raw.Labels == nil {
		return map[string]string{}, nil
	}
	return raw.Labels, nil
}

// resolveKnowledgeContentLabelsPath finds config/knowledge-content/labels-<lang>.toml
// relative to the repo root, located the same way resolveKbConfigFilePath
// locates config.toml (walk up from cwd looking for config.toml). The
// KNOWLEDGE_CONTENT_LABELS_DIR env var overrides the resolved repo root,
// mirroring KB_CONFIG_FILE's role for resolveKbConfigFilePath (used by
// tests to avoid depending on the working directory).
func resolveKnowledgeContentLabelsPath(lang string) string {
	if v := strings.TrimSpace(os.Getenv("KNOWLEDGE_CONTENT_LABELS_DIR")); v != "" {
		return filepath.Join(v, "labels-"+lang+".toml")
	}

	cur, _ := os.Getwd()
	for range 6 {
		if _, err := os.Stat(filepath.Join(cur, "config.toml")); err == nil {
			return filepath.Join(cur, "config", "knowledge-content", "labels-"+lang+".toml")
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return filepath.Join("config", "knowledge-content", "labels-"+lang+".toml")
}
