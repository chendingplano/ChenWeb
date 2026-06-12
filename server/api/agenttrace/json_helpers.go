package agenttrace

import "encoding/json"

func mapFromRaw(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	return obj
}

func stringFromRaw(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	obj := mapFromRaw(raw)
	if v, ok := obj[key].(string); ok {
		return v
	}
	return ""
}

func tokenUsageFromRaw(raw json.RawMessage, key string) TokenUsage {
	if len(raw) == 0 {
		return TokenUsage{}
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return TokenUsage{}
	}
	var usage TokenUsage
	if err := json.Unmarshal(obj[key], &usage); err != nil {
		return TokenUsage{}
	}
	return usage.withTotal()
}

func cloneMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
