package assertions

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// valueBucketMapCacheTTL mirrors valueRangeTypeMapCacheTTL: the whole table
// is small enough to cache in-process, and a per-call DB round trip for
// every "system"-method field on every metric row is not acceptable.
const valueBucketMapCacheTTL = 30 * time.Second

// valueBucketKey identifies one (dimension, normalized raw_value) pair in
// the cache -- kb.metric_value_bucket_map's PRIMARY KEY spans both columns,
// unlike kb.metric_value_range_type_map's single-dimension table, so the
// cache key must too.
type valueBucketKey struct {
	dimension string
	rawValue  string
}

// valueBucketEntry is one kb.metric_value_bucket_map row as held in the
// in-process cache.
type valueBucketEntry struct {
	CanonicalBucket string
	Status          string
}

// valueBucketMapCache holds the full kb.metric_value_bucket_map table in
// memory, keyed by (dimension, raw_value). Not exported: production code
// shares defaultValueBucketMapCache via NewValueBucketMapper; tests
// construct their own so cached state never leaks between test functions.
type valueBucketMapCache struct {
	mu       sync.RWMutex
	entries  map[valueBucketKey]valueBucketEntry
	loadedAt time.Time
}

var defaultValueBucketMapCache = &valueBucketMapCache{}

func (c *valueBucketMapCache) fresh(ctx context.Context, db *sql.DB) (map[valueBucketKey]valueBucketEntry, error) {
	c.mu.RLock()
	if c.entries != nil && time.Since(c.loadedAt) < valueBucketMapCacheTTL {
		entries := c.entries
		c.mu.RUnlock()
		return entries, nil
	}
	c.mu.RUnlock()
	return c.reload(ctx, db)
}

func (c *valueBucketMapCache) reload(ctx context.Context, db *sql.DB) (map[valueBucketKey]valueBucketEntry, error) {
	rows, err := db.QueryContext(ctx, `SELECT dimension, raw_value, canonical_bucket, status FROM kb.metric_value_bucket_map`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make(map[valueBucketKey]valueBucketEntry)
	for rows.Next() {
		var dimension, rawValue, status string
		var bucket sql.NullString
		if err := rows.Scan(&dimension, &rawValue, &bucket, &status); err != nil {
			return nil, err
		}
		entries[valueBucketKey{dimension: dimension, rawValue: rawValue}] = valueBucketEntry{CanonicalBucket: bucket.String, Status: status}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.entries = entries
	c.loadedAt = time.Now()
	c.mu.Unlock()
	return entries, nil
}

func (c *valueBucketMapCache) invalidate() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()
}

// InvalidateValueBucketMapCache clears the in-process
// kb.metric_value_bucket_map cache so the next Lookup re-reads the table
// instead of serving a stale entry for up to the 30s TTL. Callers outside
// this package that write to the table directly (e.g. an admin correction
// handler) must call this after committing.
func InvalidateValueBucketMapCache() {
	defaultValueBucketMapCache.invalidate()
}

// ValueBucketMapper is the "system"-method resolve-or-propose mechanism for
// closed-vocabulary classification fields (openspec change
// governed-property-normalization) -- structurally identical to
// ValueRangeTypeMapper, generalized by one dimension column so more than one
// field can share the table. Construct via NewValueBucketMapper in
// production; the zero-value (nil cache) is only valid in tests that supply
// their own cache field.
type ValueBucketMapper struct {
	DB    *sql.DB
	cache *valueBucketMapCache
}

// NewValueBucketMapper returns a mapper backed by the shared, process-wide
// table cache.
func NewValueBucketMapper(db *sql.DB) ValueBucketMapper {
	return ValueBucketMapper{DB: db, cache: defaultValueBucketMapCache}
}

// Lookup classifies a raw value against kb.metric_value_bucket_map for one
// dimension. canonicalBucket is non-empty and authoritative ONLY when
// status == "approved" -- mirrors ValueRangeTypeMapper.Lookup's contract
// exactly. Unlike that table, no best-effort bucket guess is seeded on
// insert: there is no generic, dimension-agnostic way to guess a canonical
// bucket, so a freshly proposed row starts with canonical_bucket = NULL.
//
// status is one of:
//   - "absent"    -- raw was empty/whitespace-only; not a mapping problem.
//   - "approved"  -- a human-confirmed mapping; canonicalBucket is usable.
//   - "ambiguous" -- a human decided no canonical bucket can be inferred.
//   - "proposed"  -- never seen before (auto-inserted by this call) or seen
//     but not yet triaged; recordID is recorded as first/last-seen.
func (m ValueBucketMapper) Lookup(ctx context.Context, dimension, raw string, recordID int64) (canonicalBucket, status string, err error) {
	normalized := normalizeValueRangeTypeRaw(raw)
	if normalized == "" {
		return "", "absent", nil
	}
	if m.DB == nil {
		return "", "", fmt.Errorf("db is nil")
	}
	key := valueBucketKey{dimension: dimension, rawValue: normalized}
	entries, err := m.cache.fresh(ctx, m.DB)
	if err != nil {
		return "", "", err
	}
	if entry, ok := entries[key]; ok {
		if entry.Status == "proposed" || entry.Status == "ambiguous" {
			if err := m.touchOccurrence(ctx, dimension, normalized, recordID); err != nil {
				return "", "", err
			}
		}
		if entry.Status != "approved" {
			return "", entry.Status, nil
		}
		return entry.CanonicalBucket, entry.Status, nil
	}
	if err := m.insertProposed(ctx, dimension, normalized, recordID); err != nil {
		return "", "", err
	}
	m.cache.invalidate()
	return "", "proposed", nil
}

func (m ValueBucketMapper) touchOccurrence(ctx context.Context, dimension, rawValue string, recordID int64) error {
	_, err := m.DB.ExecContext(ctx, `
UPDATE kb.metric_value_bucket_map
   SET occurrence_count = occurrence_count + 1, last_seen_record_id = $3, modify_time = NOW()
 WHERE dimension = $1 AND raw_value = $2`, dimension, rawValue, recordID)
	return err
}

func (m ValueBucketMapper) insertProposed(ctx context.Context, dimension, rawValue string, recordID int64) error {
	_, err := m.DB.ExecContext(ctx, `
INSERT INTO kb.metric_value_bucket_map (dimension, raw_value, status, occurrence_count, first_seen_record_id, last_seen_record_id)
VALUES ($1, $2, 'proposed', 1, $3, $3)
ON CONFLICT (dimension, raw_value) DO NOTHING`, dimension, rawValue, recordID)
	return err
}
