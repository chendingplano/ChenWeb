package docprocessing

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// promptRefFromEnv returns the configured prompt reference, falling back to
// the reference captured in prompt_defaults_generated.go when the env var is
// unset.
func promptRefFromEnv(key, fallback string) string {
	if ref := strings.TrimSpace(os.Getenv(key)); ref != "" {
		return ref
	}
	if prompt, ok := embeddedPromptDefaults[key]; ok {
		return prompt.Ref
	}
	return fallback
}

func promptRefFromEnvKeys(keys []string, fallback string) string {
	for _, key := range keys {
		if ref := strings.TrimSpace(os.Getenv(key)); ref != "" {
			return ref
		}
	}
	for _, key := range keys {
		if prompt, ok := embeddedPromptDefaults[key]; ok {
			return prompt.Ref
		}
	}
	return fallback
}

func readPromptCandidate(path string) ([]byte, error) {
	contents, err := os.ReadFile(path)
	if err == nil {
		return contents, nil
	}
	base := filepath.Base(path)
	keys := make([]string, 0, len(embeddedPromptDefaults))
	for key := range embeddedPromptDefaults {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		prompt := embeddedPromptDefaults[key]
		if filepath.Base(prompt.Ref) == base {
			return []byte(prompt.Text), nil
		}
	}
	return nil, err
}
