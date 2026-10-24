package kbsearch

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/lib/pq"
)

// BM25 index maintenance (openspec bm25-index-maintenance). pg_search keeps recently written
// documents in mutable segments, which it searches without the compiled index structures; a
// partition rewritten by search reindexing can end up with all its documents there, making every
// BM25 predicate slow (summary partition, 2026-10-24: 0.6 s per predicate). VACUUM does not compact
// them; REINDEX INDEX CONCURRENTLY does, without blocking searches.

const (
	DefaultBM25MaxMutableDocs  = 500
	DefaultBM25MaxDeletedRatio = 0.5
)

// BM25 index actions recorded per index.
const (
	BM25ActionRebuilt      = "rebuilt"
	BM25ActionWouldRebuild = "would_rebuild" // dry run
	BM25ActionSkipped      = "skipped"       // under both thresholds
	BM25ActionOverLimit    = "over_limit"    // needs a rebuild, but this run's limit is used up
	BM25ActionFailed       = "failed"
)

// BM25MaintenanceOptions are the job parameters. Zero values take the defaults; Limit <= 0 means
// no limit.
type BM25MaintenanceOptions struct {
	MaxMutableDocs  int64
	MaxDeletedRatio float64
	DryRun          bool
	Limit           int
}

// BM25IndexReport is one index's counts and the decision taken.
type BM25IndexReport struct {
	Index       string `json:"index"`
	LiveDocs    int64  `json:"live_docs"`
	MutableDocs int64  `json:"mutable_docs"`
	DeletedDocs int64  `json:"deleted_docs"`
	Action      string `json:"action"`
	Reason      string `json:"reason,omitempty"`
	MsUsed      int64  `json:"ms_used,omitempty"`
	Error       string `json:"error,omitempty"`
}

// BM25MaintenanceResult summarizes one run.
type BM25MaintenanceResult struct {
	Indexes          []BM25IndexReport `json:"indexes"`
	Rebuilt          int               `json:"rebuilt"`
	Failed           int               `json:"failed"`
	DroppedLeftovers []string          `json:"dropped_leftovers,omitempty"`
	DryRun           bool              `json:"dry_run"`
}

// leftoverBM25IndexesSQL finds invalid indexes left on the search partitions by an interrupted
// REINDEX CONCURRENTLY (named <index>_ccnew or <index>_ccnew<n>).
const leftoverBM25IndexesSQL = `SELECT c.relname
FROM pg_index i
JOIN pg_class c ON c.oid = i.indexrelid
JOIN pg_class t ON t.oid = i.indrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'kb' AND t.relname LIKE 'search\_artifacts\_%' AND NOT i.indisvalid AND c.relname LIKE '%\_ccnew%'
ORDER BY c.relname`

const listBM25IndexesSQL = `SELECT indexname FROM pg_indexes
WHERE schemaname = 'kb' AND tablename LIKE 'search\_artifacts\_%' AND indexdef ILIKE '%USING bm25%'
  AND indexname NOT LIKE '%\_ccnew%'
ORDER BY indexname`

const bm25IndexCountsSQL = `SELECT
	COALESCE(SUM(num_docs) FILTER (WHERE mutable), 0)::bigint,
	COALESCE(SUM(num_docs), 0)::bigint,
	COALESCE(SUM(num_deleted), 0)::bigint
FROM paradedb.index_info($1::regclass, false)`

// MaintainBM25Indexes inspects every BM25 index on the kb.search_artifacts_<type> partitions and
// rebuilds, concurrently, those whose mutable documents exceed MaxMutableDocs or whose deleted
// share exceeds MaxDeletedRatio. It fails only when the indexes cannot be listed; a failure on one
// index is recorded and the others are still processed.
func MaintainBM25Indexes(ctx context.Context, db *sql.DB, opts BM25MaintenanceOptions, logger ApiTypes.JimoLogger) (BM25MaintenanceResult, error) {
	res := BM25MaintenanceResult{DryRun: opts.DryRun, Indexes: []BM25IndexReport{}}
	if db == nil {
		return res, fmt.Errorf("db is nil")
	}
	if opts.MaxMutableDocs <= 0 {
		opts.MaxMutableDocs = DefaultBM25MaxMutableDocs
	}
	if opts.MaxDeletedRatio <= 0 {
		opts.MaxDeletedRatio = DefaultBM25MaxDeletedRatio
	}
	info := func(msg string, args ...any) {
		if logger != nil {
			logger.Info(msg, args...)
		}
	}
	warn := func(msg string, args ...any) {
		if logger != nil {
			logger.Warn(msg, args...)
		}
	}

	leftovers, err := queryNames(ctx, db, leftoverBM25IndexesSQL)
	if err != nil {
		return res, fmt.Errorf("list leftover bm25 indexes: %w", err)
	}
	for _, name := range leftovers {
		if opts.DryRun {
			info("bm25 maintenance: leftover index found (dry run)", "index", name)
			continue
		}
		if _, err := db.ExecContext(ctx, "DROP INDEX CONCURRENTLY IF EXISTS kb."+pq.QuoteIdentifier(name)); err != nil {
			warn("bm25 maintenance: drop leftover index failed", "index", name, "error", err)
			continue
		}
		res.DroppedLeftovers = append(res.DroppedLeftovers, name)
		info("bm25 maintenance: dropped leftover index", "index", name)
	}

	indexes, err := queryNames(ctx, db, listBM25IndexesSQL)
	if err != nil {
		return res, fmt.Errorf("list bm25 indexes: %w", err)
	}
	rebuilds := 0
	for _, name := range indexes {
		qualified := "kb." + pq.QuoteIdentifier(name)
		rep := BM25IndexReport{Index: name}
		if err := db.QueryRowContext(ctx, bm25IndexCountsSQL, qualified).Scan(&rep.MutableDocs, &rep.LiveDocs, &rep.DeletedDocs); err != nil {
			rep.Action, rep.Error = BM25ActionFailed, "index_info: "+err.Error()
			res.Failed++
			res.Indexes = append(res.Indexes, rep)
			warn("bm25 maintenance: index_info failed", "index", name, "error", err)
			continue
		}
		rep.Reason = bm25RebuildReason(rep, opts)
		switch {
		case rep.Reason == "":
			rep.Action = BM25ActionSkipped
		case opts.DryRun:
			rep.Action = BM25ActionWouldRebuild
		case opts.Limit > 0 && rebuilds >= opts.Limit:
			rep.Action = BM25ActionOverLimit
		default:
			rebuilds++
			start := time.Now()
			_, err := db.ExecContext(ctx, "REINDEX INDEX CONCURRENTLY "+qualified)
			rep.MsUsed = time.Since(start).Milliseconds()
			if err != nil {
				rep.Action, rep.Error = BM25ActionFailed, err.Error()
				res.Failed++
				warn("bm25 maintenance: rebuild failed", "index", name, "reason", rep.Reason, "ms_used", rep.MsUsed, "error", err)
			} else {
				rep.Action = BM25ActionRebuilt
				res.Rebuilt++
			}
		}
		info("bm25 maintenance: index checked", "index", name, "action", rep.Action, "reason", rep.Reason,
			"live_docs", rep.LiveDocs, "mutable_docs", rep.MutableDocs, "deleted_docs", rep.DeletedDocs, "ms_used", rep.MsUsed)
		res.Indexes = append(res.Indexes, rep)
	}
	return res, nil
}

// bm25RebuildReason returns why an index needs a rebuild, or "" when it does not.
func bm25RebuildReason(rep BM25IndexReport, opts BM25MaintenanceOptions) string {
	var reasons []string
	if rep.MutableDocs > opts.MaxMutableDocs {
		reasons = append(reasons, fmt.Sprintf("mutable_docs %d > %d", rep.MutableDocs, opts.MaxMutableDocs))
	}
	if total := rep.LiveDocs + rep.DeletedDocs; total > 0 {
		if ratio := float64(rep.DeletedDocs) / float64(total); ratio > opts.MaxDeletedRatio {
			reasons = append(reasons, fmt.Sprintf("deleted ratio %.2f > %.2f", ratio, opts.MaxDeletedRatio))
		}
	}
	return strings.Join(reasons, "; ")
}

func queryNames(ctx context.Context, db *sql.DB, query string) ([]string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
