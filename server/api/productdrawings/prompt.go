package productdrawings

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const internalPartsInstruction = `

IMPORTANT EXPLODED-VIEW REQUIREMENT: Do not show only the exterior product. Create a true cutaway/exploded technical assembly. Move or make the outer shell transparent and separate the housing panels so the internal components are visibly exposed. Show each major part as a distinct object with clear spacing between parts, including the power/control electronics, motors or pumps, valves, sensors, filters, tubing, connectors, fasteners, and structural frame appropriate to the requested product. Use numbered callouts with thin leader lines pointing to the actual parts, and include a clean parts legend. Make the internal construction mechanically plausible, legible, and easy to inspect.`

func buildDrawingPrompt(prompt string) string {
	return strings.TrimSpace(prompt) + internalPartsInstruction
}

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
