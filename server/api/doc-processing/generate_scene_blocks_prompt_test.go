package docprocessing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSceneBlocksPromptFromEnvDefaultsToEnrichmentPrompt(t *testing.T) {
	prevPrompt := os.Getenv("EXTRACT_SCENE_CANDIDATES_PROMPT")
	prevDir := os.Getenv("PROMPT_DIR")
	t.Cleanup(func() {
		_ = os.Setenv("EXTRACT_SCENE_CANDIDATES_PROMPT", prevPrompt)
		_ = os.Setenv("PROMPT_DIR", prevDir)
	})

	_ = os.Unsetenv("EXTRACT_SCENE_CANDIDATES_PROMPT")
	_ = os.Unsetenv("PROMPT_DIR")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir repo root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	_, promptRef, promptErr := loadSceneBlocksPromptFromEnv()
	if promptErr != nil {
		t.Fatalf("loadSceneBlocksPromptFromEnv: %v", promptErr)
	}
	// With no env var set the default is the embedded one (generated from mise.local.toml),
	// else the code's fallback.
	want := "prompt-enrich-scene-blocks-v1.md"
	if d, ok := embeddedPromptDefaults["ENRICH_SCENE_BLOCKS_PROMPT"]; ok {
		want = d.Ref
	}
	if got := strings.TrimSpace(promptRef); got != want {
		t.Fatalf("promptRef=%q, want %q", got, want)
	}
}
