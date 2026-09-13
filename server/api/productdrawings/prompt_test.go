package productdrawings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeExplodedViewTemplate(t *testing.T, dir string) {
	t.Helper()
	tmpl := "Draw a 3D exploded technical illustration of {{PRODUCT_NAME}}.\n\n" +
		"IMPORTANT EXPLODED-VIEW REQUIREMENT: ... clear spacing between parts." +
		"{{COMPONENTS_SENTENCE}} Use numbered callouts ... parts legend. " +
		"Make the internal construction mechanically plausible, legible, and easy to inspect.\n"
	if err := os.WriteFile(filepath.Join(dir, explodedViewPromptFileName), []byte(tmpl), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestComposeExplodedViewPromptWithComponents(t *testing.T) {
	dir := t.TempDir()
	writeExplodedViewTemplate(t, dir)

	got, err := ComposeExplodedViewPrompt(dir, "血压计", []string{"控制按钮", "电路板", "袖带"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "Draw a 3D exploded technical illustration of 血压计.") {
		t.Fatalf("prompt does not start with the expected product-name sentence: %q", got)
	}
	if !strings.Contains(got, "For instance, for a 血压计, it should include the 控制按钮, 电路板, 袖带.") {
		t.Fatalf("prompt missing components sentence: %q", got)
	}
}

func TestComposeExplodedViewPromptWithoutComponents(t *testing.T) {
	dir := t.TempDir()
	writeExplodedViewTemplate(t, dir)

	got, err := ComposeExplodedViewPrompt(dir, "thermos cup", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "For instance") {
		t.Fatalf("prompt should omit the components sentence when none are given: %q", got)
	}
	if strings.Contains(got, "{{") {
		t.Fatalf("prompt has unfilled placeholders: %q", got)
	}
}

func TestComposeExplodedViewPromptMissingTemplate(t *testing.T) {
	dir := t.TempDir()
	if _, err := ComposeExplodedViewPrompt(dir, "thermos cup", nil); err == nil {
		t.Fatal("expected an error when the template file is missing")
	}
}
