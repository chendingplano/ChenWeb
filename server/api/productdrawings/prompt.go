package productdrawings

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func loadPrompt(promptDir, subject string) (string, error) {
	if strings.ToLower(strings.TrimSpace(subject)) != "ventilator" {
		return "", fmt.Errorf("unsupported subject %q", subject)
	}
	path := filepath.Join(promptDir, promptFileName)
	prompt, err := os.ReadFile(path)
	if err != nil && promptDir == "prompts" {
		path = filepath.Join("..", promptDir, promptFileName)
		prompt, err = os.ReadFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("read canonical prompt: %w", err)
	}
	if strings.TrimSpace(string(prompt)) == "" {
		return "", fmt.Errorf("canonical prompt is empty")
	}
	return string(prompt), nil
}
