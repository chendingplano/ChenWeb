package docbuilder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Source is one directory published as a document tree, addressed by Key in the API.
type Source struct {
	Key   string `toml:"key"`
	Dir   string `toml:"dir"` // absolute, or relative to the config file
	Title string `toml:"title"`
}

type sourcesFile struct {
	Source []Source `toml:"source"`
}

// LoadSources reads a TOML file of [[source]] tables. A missing file means no sources.
func LoadSources(path string) (map[string]Source, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]Source{}, nil
	}
	if err != nil {
		return nil, err
	}
	var f sourcesFile
	if err := toml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	out := map[string]Source{}
	for _, s := range f.Source {
		s.Key = strings.TrimSpace(s.Key)
		if s.Key == "" || strings.TrimSpace(s.Dir) == "" {
			return nil, fmt.Errorf("%s: every [[source]] needs key and dir", path)
		}
		if _, dup := out[s.Key]; dup {
			return nil, fmt.Errorf("%s: duplicate source key %q", path, s.Key)
		}
		if !filepath.IsAbs(s.Dir) {
			s.Dir = filepath.Join(filepath.Dir(path), s.Dir)
		}
		out[s.Key] = s
	}
	return out, nil
}
