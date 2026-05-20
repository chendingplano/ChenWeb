package docprocessing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSceneBlocksPromptFromEnvDefaultsToV2Prompt(t *testing.T) {
	prevPrompt := os.Getenv("EXTRACT_SCENE_BLOCKS_PROMPT")
	prevDir := os.Getenv("PROMPT_DIR")
	t.Cleanup(func() {
		_ = os.Setenv("EXTRACT_SCENE_BLOCKS_PROMPT", prevPrompt)
		_ = os.Setenv("PROMPT_DIR", prevDir)
	})

	_ = os.Unsetenv("EXTRACT_SCENE_BLOCKS_PROMPT")
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
	if got, want := strings.TrimSpace(promptRef), "prompt-generate-scene-blocks-v2.md"; got != want {
		t.Fatalf("promptRef=%q, want %q", got, want)
	}
}
