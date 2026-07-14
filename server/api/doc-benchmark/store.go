package docbenchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SQLStore is the persistence boundary for benchmark lifecycle state.
type SQLStore struct{ DB *sql.DB }

var ErrNotFound = sql.ErrNoRows

func canonicalJSON(v any) ([]byte, error) {
	if v == nil {
		return []byte("{}"), nil
	}
	if b, ok := v.(json.RawMessage); ok {
		var x any
		if err := json.Unmarshal(b, &x); err != nil {
			return nil, err
		}
		return json.Marshal(x)
	}
	if b, ok := v.([]byte); ok {
		var x any
		if err := json.Unmarshal(b, &x); err != nil {
			return nil, err
		}
		return json.Marshal(x)
	}
	b, e := json.Marshal(v)
	if e != nil {
		return nil, e
	}
	var x any
	if e = json.Unmarshal(b, &x); e != nil {
		return nil, e
	}
	return json.Marshal(x)
}
func utc(t time.Time) time.Time { return t.UTC() }
func checkDB(s SQLStore) error {
	if s.DB == nil {
		return errors.New("benchmark store: nil database")
	}
	return nil
}

/*
func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
*/

func affected(res sql.Result) error {
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return fmt.Errorf("benchmark store: expected one row affected, got %d", n)
	}
	return nil
}

func txctx(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
