// Package llmrun correlates all LLM calls in one application execution.
package llmrun

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/idmanager"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/google/uuid"
)

const Sequence = "llm_run_id"

var managers sync.Map
var logger = loggerutil.CreateDefaultLogger("20261108-583")

// Next returns zero on failure; the execution must keep that result throughout.
func Next(ctx context.Context) int64 {
	db := ApiTypes.SharedDBHandle
	if db == nil {
		logger.Warn("allocate LLM run ID failed: shared database unavailable")
		return 0
	}
	cached, ok := managers.Load(db)
	if !ok {
		host, _ := os.Hostname()
		manager, err := idmanager.New(db, fmt.Sprintf("chenweb@%s:%d", host, os.Getpid()))
		if err != nil {
			logger.Warn("create LLM run ID manager failed", "error", err)
			return 0
		}
		cached, _ = managers.LoadOrStore(db, manager)
	}
	id, err := cached.(*idmanager.Manager).Next(ctx, Sequence)
	if err != nil {
		logger.Warn("allocate LLM run ID failed", "sequence", Sequence, "error", err)
		return 0
	}
	return id
}

// Stamp adds a stable execution identity before publishing to a queue. A fresh
// user request must start with a fresh payload; redelivery keeps this field.
func Stamp(payload []byte) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, fmt.Errorf("execution payload must be an object")
	}
	var key string
	_ = json.Unmarshal(raw["llm_execution_key"], &key)
	if key == "" {
		raw["llm_execution_key"], _ = json.Marshal(uuid.NewString())
	}
	return json.Marshal(raw)
}

// Key scopes a queued command to a document, so batch commands allocate one ID
// per document while redelivery retains the same identity.
func Key(payload []byte, recordID int64) string {
	var raw struct {
		Key string `json:"llm_execution_key"`
	}
	_ = json.Unmarshal(payload, &raw)
	if raw.Key == "" {
		return ""
	}
	return fmt.Sprintf("%s/%d", raw.Key, recordID)
}

// Resolve persists the allocation decision, including zero, before any LLM
// calls. Conflicting deliveries use the winner's value; gaps are harmless.
func Resolve(ctx context.Context, db *sql.DB, key string) (int64, error) {
	// Direct synchronous calls without a queue identity have no redelivery.
	if key == "" {
		return Next(ctx), nil
	}
	if db == nil {
		return 0, fmt.Errorf("LLM execution binding database is nil")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		logger.Warn("begin LLM execution failed", "error", err)
		return 0, fmt.Errorf("LLM execution binding: %w", err)
	}
	defer tx.Rollback()
	// Serialize matching deliveries before allocating. Independent execution
	// keys proceed concurrently; hash collisions only cause extra waiting.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
		logger.Warn("lock LLM execution failed", "error", err)
		return 0, fmt.Errorf("LLM execution binding: %w", err)
	}
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT llm_run_id FROM kb.llm_executions WHERE execution_key = $1`, key).Scan(&id)
	if err == sql.ErrNoRows {
		// A caller may supply the same small pool for project and shared data.
		// Bound waiting for the allocator's second connection while holding this
		// transaction, then persist zero with the original context.
		allocationCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		id = Next(allocationCtx)
		cancel()
		_, err = tx.ExecContext(ctx, `INSERT INTO kb.llm_executions (execution_key, llm_run_id) VALUES ($1, $2)`, key, id)
	}
	if err != nil {
		logger.Warn("persist LLM execution failed", "error", err)
		return 0, fmt.Errorf("LLM execution binding: %w", err)
	}
	if err = tx.Commit(); err != nil {
		logger.Warn("commit LLM execution failed", "error", err)
		return 0, fmt.Errorf("LLM execution binding: %w", err)
	}
	return id, nil
}

// Inherit carries a queued batch command's identity into one child document.
func Inherit(parent, child []byte) ([]byte, error) {
	var source, target map[string]json.RawMessage
	if err := json.Unmarshal(parent, &source); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(child, &target); err != nil {
		return nil, err
	}
	if key, ok := source["llm_execution_key"]; ok {
		target["llm_execution_key"] = key
	}
	return json.Marshal(target)
}

// QueuePayload supplies a deterministic identity for legacy queued messages
// that predate producer stamping. Stream plus sequence is stable on redelivery.
func QueuePayload(payload []byte, stream string, sequence uint64) []byte {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil || raw == nil {
		return payload
	}
	if _, ok := raw["llm_execution_key"]; !ok {
		raw["llm_execution_key"], _ = json.Marshal(fmt.Sprintf("jetstream/%s/%d", stream, sequence))
	}
	result, err := json.Marshal(raw)
	if err != nil {
		return payload
	}
	return result
}

// Provision bootstraps the application sequence after project and shared schema
// migrations complete. Stop legacy writers before cutover; repeated startup
// preserves the sequence's existing range and reservation history.
func Provision(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("LLM sequence provisioning database is nil")
	}
	_, err := db.ExecContext(ctx, `INSERT INTO shared.id_mgr
    (id_name, id_desc, next_id, start_id, max_id, allocate_size, allow_reuse, creator)
SELECT 'llm_run_id', 'ChenWeb LLM execution correlation',
       GREATEST(COALESCE(MAX(run_id), 0), 0) + 1,
       GREATEST(COALESCE(MAX(run_id), 0), 0) + 1,
       9223372036854775806, 100, FALSE, 'chenweb-startup'
FROM public.llm_usage_event
ON CONFLICT (id_name) DO NOTHING;`)
	if err != nil {
		logger.Error("provision LLM run sequence failed", "sequence", Sequence, "error", err)
		return fmt.Errorf("provision LLM run sequence: %w", err)
	}
	logger.Info("LLM run sequence provisioned", "sequence", Sequence)
	return nil
}
