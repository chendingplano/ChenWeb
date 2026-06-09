package docprocessing

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type StartDocProcessingEvent struct {
	RecordIDs      []int64
	Filename       string
	All            string
	DocProcessors  []string
	FailedProcOnly bool
}

func ParseStartDocProcessingEvent(payload []byte) (StartDocProcessingEvent, error) {
	raw, err := decodeEventPayload(payload)
	if err != nil {
		return StartDocProcessingEvent{}, fmt.Errorf("decode start doc-processing event: %w", err)
	}

	recordIDs, err := parseRecordIDs(raw)
	if err != nil {
		return StartDocProcessingEvent{}, err
	}
	_, hasRecordIDs := firstPresent(raw, "record_ids", "record-ids")
	_, hasDocProcessors := firstPresent(raw, "doc-processors", "doc_processors", "operation")
	_, hasFailedProcOnly := firstPresent(raw, "failed-proc-only", "failed_proc_only")
	if len(recordIDs) == 0 {
		if rid, ok := raw["record_id"]; ok {
			id, err := parseRecordID(rid)
			if err != nil {
				return StartDocProcessingEvent{}, err
			}
			recordIDs = []int64{id}
		}
	}

	all := strings.ToLower(strings.TrimSpace(asString(raw["all"])))
	switch all {
	case "", "parsed", "failed-procs", "with-failed-procs":
	default:
		return StartDocProcessingEvent{}, fmt.Errorf("invalid all value %q", all)
	}
	if all == "" && len(recordIDs) == 0 {
		return StartDocProcessingEvent{}, errors.New("all or record_ids is required")
	}

	autoMode := all == "" && len(recordIDs) == 1 && !hasRecordIDs && !hasDocProcessors && !hasFailedProcOnly
	failedProcOnly := !autoMode
	if v, ok := firstPresent(raw, "failed-proc-only", "failed_proc_only"); ok {
		b, err := asBool(v, true)
		if err != nil {
			return StartDocProcessingEvent{}, fmt.Errorf("invalid failed-proc-only: %w", err)
		}
		failedProcOnly = b
	}

	return StartDocProcessingEvent{
		RecordIDs:      recordIDs,
		Filename:       firstNonEmptyTrimmed(asString(raw["filename"]), asString(raw["line_file_filename"])),
		All:            all,
		DocProcessors:  parseOperations(firstPresentValue(raw, "doc-processors", "doc_processors", "operation")),
		FailedProcOnly: failedProcOnly,
	}, nil
}

func parseRecordIDs(raw map[string]any) ([]int64, error) {
	v, ok := firstPresent(raw, "record_ids", "record-ids")
	if !ok {
		return nil, nil
	}
	seen := map[int64]struct{}{}
	out := make([]int64, 0, 8)
	appendID := func(id int64) error {
		if id <= 0 {
			return fmt.Errorf("record id must be positive: %d", id)
		}
		if _, ok := seen[id]; ok {
			return nil
		}
		seen[id] = struct{}{}
		out = append(out, id)
		return nil
	}
	parseOne := func(item any) error {
		switch x := item.(type) {
		case float64:
			if x != float64(int64(x)) {
				return fmt.Errorf("record id must be an integer: %v", x)
			}
			return appendID(int64(x))
		case json.Number:
			id, err := x.Int64()
			if err != nil {
				return fmt.Errorf("invalid record id %q: %w", x.String(), err)
			}
			return appendID(id)
		case string:
			return parseRecordIDToken(x, appendID)
		default:
			return parseRecordIDToken(asString(item), appendID)
		}
	}

	switch x := v.(type) {
	case []any:
		for _, item := range x {
			if err := parseOne(item); err != nil {
				return nil, err
			}
		}
	case string:
		for _, part := range strings.Split(x, ",") {
			if err := parseOne(part); err != nil {
				return nil, err
			}
		}
	default:
		if err := parseOne(v); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func parseRecordIDToken(raw string, appendID func(int64) error) error {
	token := strings.TrimSpace(raw)
	if token == "" {
		return nil
	}
	if strings.Contains(token, "-") {
		parts := strings.Split(token, "-")
		if len(parts) != 2 {
			return fmt.Errorf("invalid record id range %q", token)
		}
		start, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid range start %q: %w", token, err)
		}
		end, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid range end %q: %w", token, err)
		}
		if start > end {
			return fmt.Errorf("invalid descending record id range %q", token)
		}
		for id := start; id <= end; id++ {
			if err := appendID(id); err != nil {
				return err
			}
		}
		return nil
	}
	id, err := strconv.ParseInt(token, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid record id %q: %w", token, err)
	}
	return appendID(id)
}

func firstPresent(raw map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if v, ok := raw[key]; ok {
			return v, true
		}
	}
	return nil, false
}

func firstPresentValue(raw map[string]any, keys ...string) any {
	v, _ := firstPresent(raw, keys...)
	return v
}
