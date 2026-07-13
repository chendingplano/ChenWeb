package docbenchmark

import (
	"context"
	"encoding/json"
	"errors"
)

var ErrInvalidOutput = errors.New("invalid_output")

// Adapter is the narrow contract shared by production processor adapters.
type Adapter interface {
	Processor() Processor
	AllowedOverrides() map[string]any
	Applicable(ExpectedOutput) bool
	Capture(context.Context, int64) (any, error)
	Reconcile(any) (any, error)
	Cleanup(context.Context, int64) error
}

// CapturedArtifact is a byte-stable representation suitable for Workspace.Capture.
type CapturedArtifact struct {
	Name string
	Data []byte
}

func canonicalValue(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var x any
	if err := json.Unmarshal(b, &x); err != nil {
		return nil, err
	}
	return json.Marshal(x)
}
