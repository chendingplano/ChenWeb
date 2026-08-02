package docprocessing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/pathutil"
)

const DefaultEventSubject = "kb.pdf.start-doc-processing"

type LineFileGeneratedEvent struct {
	RecordID         int64
	Filename         string
	Force            bool
	ForceClear       bool
	Type             string
	Status           string
	Operations       []string
	PipelineOverride string
}

func ParseLineFileGeneratedEvent(payload []byte) (LineFileGeneratedEvent, error) {
	raw, err := decodeEventPayload(payload)
	if err != nil {
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

	forceClear := false
	if v, ok := raw["force_clear"]; ok {
		b, bErr := asBool(v, false)
		if bErr != nil {
			return LineFileGeneratedEvent{}, fmt.Errorf("invalid force_clear: %w", bErr)
		}
		forceClear = b
	}

	return LineFileGeneratedEvent{
		RecordID:         rid,
		Filename:         firstNonEmptyTrimmed(asString(raw["filename"]), asString(raw["line_file_filename"])),
		Force:            force,
		ForceClear:       forceClear,
		Type:             strings.ToLower(strings.TrimSpace(asString(raw["type"]))),
		Status:           strings.ToLower(strings.TrimSpace(asString(raw["status"]))),
		Operations:       parseOperations(firstPresentValue(raw, "operation", "processor_override", "processor_overrides", "doc-processors", "doc_processors")),
		PipelineOverride: strings.TrimSpace(asString(raw["pipeline_override"])),
	}, nil
}

func decodeEventPayload(payload []byte) (map[string]any, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil, errors.New("empty payload")
	}

	var raw map[string]any
	if err := json.Unmarshal(trimmed, &raw); err == nil {
		return raw, nil
	}

	var encoded string
	if err := json.Unmarshal(trimmed, &encoded); err == nil {
		inner := bytes.TrimSpace([]byte(encoded))
		if len(inner) == 0 {
			return nil, errors.New("empty string payload")
		}
		if err := json.Unmarshal(inner, &raw); err == nil {
			return raw, nil
		}
	}

	return nil, errors.New(previewInvalidEventPayload(trimmed))
}

func previewInvalidEventPayload(payload []byte) string {
	const maxLen = 160
	if len(payload) > maxLen {
		payload = payload[:maxLen]
	}
	preview := strings.ReplaceAll(string(payload), "\n", "\\n")
	preview = strings.ReplaceAll(preview, "\r", "\\r")
	preview = strings.ReplaceAll(preview, "\t", "\\t")
	return fmt.Sprintf("invalid JSON payload preview=%q", preview)
}

func ShouldSkipLineFileGeneratedEvent(evt LineFileGeneratedEvent) bool {
	return skipReasonLineFileGeneratedEvent(evt) != ""
}

func skipReasonLineFileGeneratedEvent(evt LineFileGeneratedEvent) string {
	if evt.Type != "" && evt.Type != "pdf" {
		return fmt.Sprintf("type=%q", evt.Type)
	}
	if evt.Status != "" && evt.Status != "success" {
		return fmt.Sprintf("status=%q", evt.Status)
	}
	return ""
}

func ResolveInputFilePath(evt LineFileGeneratedEvent, resultFilename string, parserName string, stagingFilename string) (string, error) {
	resultPath := pathutil.ResolveDataHomePath(resultFilename)
	if strings.TrimSpace(evt.Filename) != "" {
		f := strings.TrimSpace(evt.Filename)
		if hasPathSeparator(f) {
			if !filepath.IsAbs(f) {
				return "", fmt.Errorf("filename with path must be absolute: %s", f)
			}
			return normalizeWorkingLineFilePath(filepath.Clean(f)), nil
		}
		if resultPath == "" {
			return "", errors.New("missing result filename")
		}
		return normalizeWorkingLineFilePath(filepath.Join(filepath.Dir(resultPath), f)), nil
	}

	if resultPath == "" {
		return "", errors.New("missing result filename")
	}
	if strings.TrimSpace(parserName) == "" {
		return "", errors.New("missing parser name")
	}

	root := strings.TrimSuffix(filepath.Base(pathutil.ResolveDataHomePath(stagingFilename)), filepath.Ext(strings.TrimSpace(stagingFilename)))
	if root == "" {
		root = strings.TrimSuffix(filepath.Base(resultPath), filepath.Ext(resultPath))
	}
	if root == "" {
		root = "result"
	}
	return normalizeWorkingLineFilePath(filepath.Join(filepath.Dir(resultPath), root+"_"+strings.TrimSpace(parserName)+".txt")), nil
}

func normalizeWorkingLineFilePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if strings.HasSuffix(strings.ToLower(trimmed), ".origin") {
		return trimmed[:len(trimmed)-len(".origin")] + ".txt"
	}
	return trimmed
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
	case "chunked":
		return "chunking"
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

func firstNonEmptyTrimmed(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
