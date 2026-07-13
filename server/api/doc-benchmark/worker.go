package docbenchmark

// worker.go defines the deliberately small, versioned JSON-lines protocol used
// between the benchmark orchestrator and an isolated worker process.  Stdout is
// reserved for these frames; diagnostic output belongs on stderr.

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

const WorkerProtocolVersion = 1

type WorkerRequest struct {
	Version        int               `json:"version"`
	Type           string            `json:"type"`
	Variant        ExperimentVariant `json:"variant,omitempty"`
	RunID          string            `json:"run_id,omitempty"`
	CaseID         string            `json:"case_id,omitempty"`
	ConfigHash     string            `json:"config_hash,omitempty"`
	ExecutableHash string            `json:"executable_hash,omitempty"`
}

type WorkerMessage struct {
	Version        int                  `json:"version"`
	Type           string               `json:"type"`
	Config         ResolvedWorkerConfig `json:"config,omitempty"`
	ConfigHash     string               `json:"config_hash,omitempty"`
	ExecutableHash string               `json:"executable_hash,omitempty"`
	RunID          string               `json:"run_id,omitempty"`
	CaseID         string               `json:"case_id,omitempty"`
	Error          *WorkerFailure       `json:"error,omitempty"`
}

type ResolvedWorkerConfig struct {
	Snapshot json.RawMessage `json:"snapshot"`
	Hash     string          `json:"hash"`
}

type WorkerFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type WorkerInitializer func(context.Context, ExperimentVariant) (ResolvedWorkerConfig, error)
type WorkerCaseRunner func(context.Context, string) error

type WorkerServer struct {
	Initialize   WorkerInitializer
	RunCase      WorkerCaseRunner
	MaxLineBytes int
}

func (s WorkerServer) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	if s.Initialize == nil {
		return errors.New("worker: initializer is required")
	}
	dec := json.NewDecoder(bufio.NewReader(in))
	dec.DisallowUnknownFields()
	enc := json.NewEncoder(out)
	var req WorkerRequest
	if err := dec.Decode(&req); err != nil {
		return fmt.Errorf("worker initialize: %w", err)
	}
	if req.Version != WorkerProtocolVersion || req.Type != "initialize" {
		return errors.New("worker: invalid initialize request")
	}
	cfg, err := s.Initialize(ctx, req.Variant)
	if err != nil {
		_ = enc.Encode(WorkerMessage{Version: WorkerProtocolVersion, Type: "failure", Error: &WorkerFailure{Code: "initialize_failed", Message: err.Error()}})
		return err
	}
	if cfg.Hash == "" {
		return errors.New("worker: empty config hash")
	}
	if req.ConfigHash != "" && req.ConfigHash != cfg.Hash {
		return errors.New("worker: config hash mismatch")
	}
	if err := enc.Encode(WorkerMessage{Version: WorkerProtocolVersion, Type: "ready", Config: cfg, ConfigHash: cfg.Hash, ExecutableHash: req.ExecutableHash}); err != nil {
		return err
	}
	var auth WorkerRequest
	if err := dec.Decode(&auth); err != nil {
		return fmt.Errorf("worker authorize: %w", err)
	}
	if auth.Version != WorkerProtocolVersion || auth.Type != "authorize" || auth.RunID == "" {
		return errors.New("worker: invalid authorization")
	}
	if err := enc.Encode(WorkerMessage{Version: WorkerProtocolVersion, Type: "authorized", RunID: auth.RunID}); err != nil {
		return err
	}
	if s.RunCase == nil {
		return nil
	}
	for {
		var r WorkerRequest
		if err := dec.Decode(&r); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("worker request: %w", err)
		}
		if r.Version != WorkerProtocolVersion {
			return errors.New("worker: protocol version mismatch")
		}
		switch r.Type {
		case "case":
			if r.CaseID == "" {
				return errors.New("worker: empty case id")
			}
			_ = enc.Encode(WorkerMessage{Version: WorkerProtocolVersion, Type: "heartbeat", CaseID: r.CaseID})
			e := s.RunCase(ctx, r.CaseID)
			m := WorkerMessage{Version: WorkerProtocolVersion, Type: "result", CaseID: r.CaseID}
			if e != nil {
				m.Error = &WorkerFailure{Code: "case_failed", Message: e.Error()}
			}
			if err := enc.Encode(m); err != nil {
				return err
			}
		case "shutdown":
			return nil
		default:
			return fmt.Errorf("worker: unknown request type %q", r.Type)
		}
	}
}

// ExecutableSHA256 returns the provenance hash of the running executable.
func ExecutableSHA256(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

type lockedEncoder struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func (e *lockedEncoder) Encode(v any) error { e.mu.Lock(); defer e.mu.Unlock(); return e.enc.Encode(v) }
