package docbenchmark

import (
	"context"
	"database/sql"
)

// sqlCleanupTx scopes cleanup statements to one execution attempt.
type sqlCleanupTx struct {
	tx    *sql.Tx
	owner string
}

func (c *sqlCleanupTx) DeleteProductionRows() error {
	if _, err := c.tx.ExecContext(context.Background(), `DELETE FROM kb.benchmark_scores WHERE attempt_id=$1`, c.owner); err != nil {
		return err
	}
	// Verified artifacts are immutable and must remain available as evidence.
	_, err := c.tx.ExecContext(context.Background(), `DELETE FROM kb.benchmark_artifacts WHERE attempt_id=$1 AND verified=false`, c.owner)
	return err
}

func (c *sqlCleanupTx) DeleteInput() error {
	var inputID sql.NullInt64
	if err := c.tx.QueryRowContext(context.Background(), `SELECT input_record_id FROM kb.benchmark_workspaces WHERE execution_attempt_id=$1 FOR UPDATE`, c.owner).Scan(&inputID); err != nil {
		return err
	}
	if !inputID.Valid {
		return nil
	}
	_, err := c.tx.ExecContext(context.Background(), `DELETE FROM kb.inputs WHERE id=$1`, inputID.Int64)
	return err
}

func (c *sqlCleanupTx) MarkState(state string, cause error) error {
	var msg *string
	if cause != nil {
		s := cause.Error()
		msg = &s
	}
	_, err := c.tx.ExecContext(context.Background(), `UPDATE kb.benchmark_workspaces SET cleanup_state=$2, cleanup_error=$3, cleaned_at=CASE WHEN $2='cleaned' THEN now() ELSE cleaned_at END WHERE execution_attempt_id=$1`, c.owner, state, msg)
	return err
}

func (s SQLStore) CleanupTransaction(ctx context.Context, owner string, fn func(CleanupTx) error) error {
	if err := checkDB(s); err != nil {
		return err
	}
	if fn == nil {
		return ErrUnsafePath
	}
	tx, err := s.DB.BeginTx(txctx(ctx), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(txctx(ctx), `SELECT id FROM kb.benchmark_workspaces WHERE execution_attempt_id=$1 FOR UPDATE`, owner); err != nil {
		return err
	}
	if err = fn(&sqlCleanupTx{tx: tx, owner: owner}); err != nil {
		return err
	}
	return tx.Commit()
}
