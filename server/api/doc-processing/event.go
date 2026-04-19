package docprocessing

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

const DefaultEventSubject = "kb.line-file-generated"

type LineFileGeneratedEvent struct {
	RecordID   int64
	Filename   string
	Force      bool
	Type       string
	Status     string
	Operations []string
}

func ParseLineFileGeneratedEvent(payload []byte) (LineFileGeneratedEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return LineFileGeneratedEvent{}, fmt.Errorf("decode event: %w", err)
	}

	rid, err := parseRecordID(raw["record_id"])
	if err != nil {
		return LineFileGeneratedEvent{}, err
	}

	force := true
	if v, ok := raw["force"]; ok {
		b, bErr := asBool(v, true)
		if bErr != nil {
			return LineFileGeneratedEvent{}, fmt.Errorf("invalid force: %w", bErr)
		}
		force = b
	}

	return LineFileGeneratedEvent{
		RecordID:   rid,
		Filename:   strings.TrimSpace(asString(raw["filename"])),
		Force:      force,
		Type:       strings.ToLower(strings.TrimSpace(asString(raw["type"]))),
		Status:     strings.ToLower(strings.TrimSpace(asString(raw["status"]))),
		Operations: parseOperations(raw["operation"]),
	}, nil
}

func ShouldSkipLineFileGeneratedEvent(evt LineFileGeneratedEvent) bool {
	if evt.Type != "" && evt.Type != "pdf" {
		return true
	}
	if evt.Status != "" && evt.Status != "success" {
		return true
	}
	return false
}

func ResolveInputFilePath(evt LineFileGeneratedEvent, resultFilename string, parserName string, stagingFilename string) (string, error) {
	resultPath := strings.TrimSpace(resultFilename)
	if strings.TrimSpace(evt.Filename) != "" {
		f := strings.TrimSpace(evt.Filename)
		if hasPathSeparator(f) {
			if !filepath.IsAbs(f) {
				return "", fmt.Errorf("filename with path must be absolute: %s", f)
			}
			return filepath.Clean(f), nil
		}
		if resultPath == "" {
			return "", errors.New("missing result filename")
		}
		return filepath.Join(filepath.Dir(resultPath), f), nil
	}

	if resultPath == "" {
		return "", errors.New("missing result filename")
	}
	if strings.TrimSpace(parserName) == "" {
		return "", errors.New("missing parser name")
	}

	root := strings.TrimSuffix(filepath.Base(strings.TrimSpace(stagingFilename)), filepath.Ext(strings.TrimSpace(stagingFilename)))
	if root == "" {
		root = strings.TrimSuffix(filepath.Base(resultPath), filepath.Ext(resultPath))
	}
	if root == "" {
		root = "result"
	}
	return filepath.Join(filepath.Dir(resultPath), root+"_"+strings.TrimSpace(parserName)+".txt"), nil
}

func parseOperations(v any) []string {
	if v == nil {
		return nil
	}
	out := make([]string, 0, 4)
	appendOp := func(raw string) {
		op := canonicalOperationName(raw)
		if op != "" {
			out = append(out, op)
		}
	}

	switch x := v.(type) {
	case string:
		for _, part := range strings.Split(x, ",") {
			appendOp(part)
		}
	case []any:
		for _, item := range x {
			appendOp(asString(item))
		}
	}
	return out
}

func canonicalOperationName(raw string) string {
	op := strings.ToLower(strings.TrimSpace(raw))
	op = strings.ReplaceAll(op, "-", "_")
	switch op {
	case "extract_metadata":
		return "extract_doc_metadata"
	default:
		return op
	}
}

func parseRecordID(v any) (int64, error) {
	s := strings.TrimSpace(asString(v))
	if s == "" {
		return 0, errors.New("missing record_id")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid record_id: %q", s)
	}
	return n, nil
}

func asBool(v any, defaultValue bool) (bool, error) {
	if v == nil {
		return defaultValue, nil
	}
	s := strings.ToLower(strings.TrimSpace(asString(v)))
	switch s {
	case "", "null", "none":
		return defaultValue, nil
	case "1", "true", "yes", "y", "on":
		return true, nil
	case "0", "false", "no", "n", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool value: %v", v)
	}
}
