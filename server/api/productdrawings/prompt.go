package productdrawings

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const explodedViewPromptFileName = "prompt-product-drawing-exploded-view-v1.md"

// ComposeExplodedViewPrompt renders the exploded-view drawing prompt template
// for productName, listing components (product-specific part names, e.g. from
// a profile's scope tree) as the parts the illustration should include. When
// components is empty, the "For instance..." sentence is omitted entirely
// rather than rendered malformed.
func ComposeExplodedViewPrompt(promptDir, productName string, components []string) (string, error) {
	tmpl, err := readPromptFile(promptDir, explodedViewPromptFileName)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(productName)
	sentence := ""
	if len(components) > 0 {
		sentence = fmt.Sprintf(" For instance, for a %s, it should include the %s.", name, strings.Join(components, ", "))
	}
	out := strings.ReplaceAll(tmpl, "{{PRODUCT_NAME}}", name)
	out = strings.ReplaceAll(out, "{{COMPONENTS_SENTENCE}}", sentence)
	return strings.TrimSpace(out), nil
}

func loadPrompt(promptDir, subject string) (string, error) {
	if strings.ToLower(strings.TrimSpace(subject)) != "ventilator" {
		return "", fmt.Errorf("unsupported subject %q", subject)
	}
	return readPromptFile(promptDir, promptFileName)
}

func readPromptFile(promptDir, fileName string) (string, error) {
	path := filepath.Join(promptDir, fileName)
	prompt, err := os.ReadFile(path)
	if err != nil && promptDir == "prompts" {
		path = filepath.Join("..", promptDir, fileName)
		prompt, err = os.ReadFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("read prompt %s: %w", fileName, err)
	}
	if strings.TrimSpace(string(prompt)) == "" {
		return "", fmt.Errorf("prompt %s is empty", fileName)
	}
	return string(prompt), nil
}
