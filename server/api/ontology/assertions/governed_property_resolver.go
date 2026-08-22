package assertions

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// governedPropertyValueMapCacheTTL mirrors valueRangeTypeMapCacheTTL: the
// whole table is small enough to cache in-process, and a per-call DB round
// trip for every identity dimension on every metric row is not acceptable
// (ADR 2026082203 design D1).
const governedPropertyValueMapCacheTTL = 30 * time.Second

// governedPropertyKey identifies one (dimension, normalized raw_value) pair
// in the cache -- kb.governed_property_value_map's PRIMARY KEY spans both
// columns, unlike kb.metric_value_range_type_map's single-dimension table,
// so the cache key must too.
type governedPropertyKey struct {
	dimension string
	rawValue  string
}

// governedPropertyEntry is one kb.governed_property_value_map row as held in
// the in-process cache.
type governedPropertyEntry struct {
	TermID string
	Status string
}

// governedPropertyValueMapCache holds the full kb.governed_property_value_map
// table in memory, keyed by (dimension, raw_value). Not exported: production
// code shares defaultGovernedPropertyValueMapCache via
// NewGovernedPropertyResolver; tests construct their own so cached state
// never leaks between test functions.
type governedPropertyValueMapCache struct {
	mu       sync.RWMutex
	entries  map[governedPropertyKey]governedPropertyEntry
	loadedAt time.Time
}

var defaultGovernedPropertyValueMapCache = &governedPropertyValueMapCache{}

func (c *governedPropertyValueMapCache) fresh(ctx context.Context, db *sql.DB) (map[governedPropertyKey]governedPropertyEntry, error) {
	c.mu.RLock()
	if c.entries != nil && time.Since(c.loadedAt) < governedPropertyValueMapCacheTTL {
		entries := c.entries
		c.mu.RUnlock()
		return entries, nil
	}
	c.mu.RUnlock()
	return c.reload(ctx, db)
}

func (c *governedPropertyValueMapCache) reload(ctx context.Context, db *sql.DB) (map[governedPropertyKey]governedPropertyEntry, error) {
	rows, err := db.QueryContext(ctx, `SELECT dimension, raw_value, term_id, status FROM kb.governed_property_value_map`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make(map[governedPropertyKey]governedPropertyEntry)
	for rows.Next() {
		var dimension, rawValue, status string
		var termID sql.NullString
		if err := rows.Scan(&dimension, &rawValue, &termID, &status); err != nil {
			return nil, err
		}
		entries[governedPropertyKey{dimension: dimension, rawValue: rawValue}] = governedPropertyEntry{TermID: termID.String, Status: status}
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

func (c *governedPropertyValueMapCache) invalidate() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()
}

// InvalidateGovernedPropertyValueMapCache clears the in-process
// kb.governed_property_value_map cache so the next Lookup re-reads the table
// instead of serving a stale entry for up to the 30s TTL. Callers outside
// this package that write to the table directly (e.g. an admin correction
// handler) must call this after committing.
func InvalidateGovernedPropertyValueMapCache() {
	defaultGovernedPropertyValueMapCache.invalidate()
}

// GovernedPropertyResolver is the generic, per-dimension resolve-or-propose
// mechanism (ADR 2026082203 DR3), generalizing ValueRangeTypeMapper's shape
// across every class-identity dimension instead of growing a new
// single-dimension table per field. Construct via
// NewGovernedPropertyResolver in production; the zero-value (nil cache) is
// only valid in tests that supply their own cache field.
type GovernedPropertyResolver struct {
	DB    *sql.DB
	cache *governedPropertyValueMapCache
}

// NewGovernedPropertyResolver returns a resolver backed by the shared,
// process-wide table cache.
func NewGovernedPropertyResolver(db *sql.DB) GovernedPropertyResolver {
	return GovernedPropertyResolver{DB: db, cache: defaultGovernedPropertyValueMapCache}
}

// Lookup resolves a raw occurrence field value against
// kb.governed_property_value_map for one dimension. termID is non-empty and
// authoritative ONLY when status == "approved" -- mirrors
// ValueRangeTypeMapper.Lookup exactly (ADR 2026081401 DR1's established
// contract, generalized). The raw value is normalized via the same floor
// rule as the value-range-type mapper (ADR 2026082203 OD1: lowercase, trim,
// '-'/' ' -> '_') so a dimension needing stronger normalization is a future,
// separately-scoped change, not a second normalization function here.
//
// status is one of:
//   - "absent"    -- raw was empty/whitespace-only; not a mapping problem.
//   - "approved"  -- a human-confirmed mapping; termID is usable.
//   - "ambiguous" -- a human decided no governed term can be inferred.
//   - "proposed"  -- never seen before (auto-inserted by this call) or seen
//     but not yet triaged; recordID is recorded as first/last-seen.
//   - "rejected"  -- a human decided this raw value should never resolve.
func (r GovernedPropertyResolver) Lookup(ctx context.Context, dimension, raw string, recordID int64) (termID, status string, err error) {
	normalized := normalizeValueRangeTypeRaw(raw)
	if normalized == "" {
		return "", "absent", nil
	}
	if r.DB == nil {
		return "", "", fmt.Errorf("db is nil")
	}
	key := governedPropertyKey{dimension: dimension, rawValue: normalized}
	entries, err := r.cache.fresh(ctx, r.DB)
	if err != nil {
		return "", "", err
	}
	if entry, ok := entries[key]; ok {
		if entry.Status == "proposed" || entry.Status == "ambiguous" {
			if err := r.touchOccurrence(ctx, dimension, normalized, recordID); err != nil {
				return "", "", err
			}
		}
		if entry.Status != "approved" {
			return "", entry.Status, nil
		}
		return entry.TermID, entry.Status, nil
	}
	if err := r.insertProposed(ctx, dimension, normalized, recordID); err != nil {
		return "", "", err
	}
	r.cache.invalidate()
	return "", "proposed", nil
}

func (r GovernedPropertyResolver) touchOccurrence(ctx context.Context, dimension, rawValue string, recordID int64) error {
	_, err := r.DB.ExecContext(ctx, `
UPDATE kb.governed_property_value_map
   SET occurrence_count = occurrence_count + 1, last_seen_record_id = $3, modify_time = NOW()
 WHERE dimension = $1 AND raw_value = $2`, dimension, rawValue, recordID)
	return err
}

func (r GovernedPropertyResolver) insertProposed(ctx context.Context, dimension, rawValue string, recordID int64) error {
	_, err := r.DB.ExecContext(ctx, `
INSERT INTO kb.governed_property_value_map (dimension, raw_value, status, occurrence_count, first_seen_record_id, last_seen_record_id)
VALUES ($1, $2, 'proposed', 1, $3, $3)
ON CONFLICT (dimension, raw_value) DO NOTHING`, dimension, rawValue, recordID)
	return err
}
