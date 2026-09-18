// Package datasync lets a deployed ChenWeb instance (the "target") pull
// specific, registered data items from another instance (the "source"),
// admin-triggered, over a token-authenticated connection the target itself
// initiates. See openspec/changes/production-data-sync for the original
// design and openspec/changes/configurable-data-sync-items for runtime-
// created items, the table_with_files kind, and cross-instance discovery.
// A row that references another synced table's row (e.g. kb.videos.image_uid
// -> kb.images.uid) must store that table's stable natural key as plain data,
// never its surrogate id -- see design.md Decision 8; no support for this is
// (or needs to be) built into this package itself.
package datasync

// SyncKind distinguishes what a sync item's rows carry. The zero value
// behaves as KindTable, so every compiled item literal written before this
// existed (kb_product_names) needs no changes.
type SyncKind string

const (
	KindTable          SyncKind = "table"
	KindTableWithFiles SyncKind = "table_with_files"
)

// SyncOrigin marks where a DB-backed (non-compiled) item's definition came
// from. Compiled Registry items have no Origin (not stored in
// kb.data_sync_items at all). See design.md Decision 6.
type SyncOrigin string

const (
	// OriginLocal: created on this instance via the admin UI.
	OriginLocal SyncOrigin = "local"
	// OriginLearned: write-through-cached from this instance's configured
	// DATA_SYNC_SOURCE_URL via item-definition discovery (client.go).
	OriginLearned SyncOrigin = "learned"
)

// TableSyncItem describes one Postgres table that can be synced: an
// admin-triggered, incremental, upsert-only pull from a source deployment
// into a target deployment.
type TableSyncItem struct {
	// ID is the stable identifier used in API paths and in kb.data_sync_state.
	ID string
	// Table is the fully-qualified source/target table name, e.g. "kb.product_names".
	Table string
	// CursorCol is the column used to detect "changed since". It must be
	// monotonically increasing and bumped on every UPDATE (see the
	// kb.set_update_time trigger for kb.product_names).
	CursorCol string
	// NaturalKey is the column set used as the upsert conflict target. It
	// must NOT include a database-local surrogate key (e.g. a BIGSERIAL id),
	// since source and target databases generate those independently.
	NaturalKey []string
	// Columns is the full set of columns to transfer, excluding any
	// surrogate primary key. Must include every column in NaturalKey and CursorCol.
	Columns []string
	// JSONColumns is the subset of Columns that are JSONB in Postgres, so
	// they can be passed through as raw JSON instead of a JSON-encoded string.
	JSONColumns []string
	// Filter, if non-empty, is a raw SQL boolean expression ANDed into the
	// source's pull query, scoping which rows this item ever touches.
	Filter string

	// Kind selects the sync behavior. Zero value ("") behaves as KindTable.
	Kind SyncKind

	// The following are only meaningful when Kind == KindTableWithFiles.

	// FileColumn is one of Columns; it holds a server-local absolute file
	// path on whichever instance is currently the source. Never trusted as
	// a target-local path directly -- see files.go.
	FileColumn string
	// FileDirEnv, if set, names the env var this instance reads to resolve
	// where it stores this item's files locally (e.g. "VIDEO_DIR").
	FileDirEnv string
	// FileDirDefaultSubdir is the fallback subdirectory under DATA_HOME_DIR
	// when FileDirEnv is unset or empty, mirroring videohandler.videoDir()'s
	// own resolution order.
	FileDirDefaultSubdir string

	// Origin is only meaningful for DB-backed items (empty for compiled
	// Registry items, which aren't stored in kb.data_sync_items at all).
	Origin SyncOrigin
}

// Registry lists every sync item known to this binary. Both the source pull
// handler and the target apply logic read the same registry, since source
// and target run the same binary.
var Registry = []TableSyncItem{
	{
		ID:        "kb_product_names",
		Table:     "kb.product_names",
		CursorCol: "update_time",
		// UNIQUE(source, seq_no, product_name) already exists on the table
		// (20260912000002_create_kb_product_names.sql).
		NaturalKey: []string{"source", "seq_no", "product_name"},
		Columns: []string{
			"seq_no", "sub_catalog", "category_l1", "category_l2", "description",
			"intended_use", "product_name", "product_name_en", "regulatory_class",
			"aliases", "keywords", "source", "notes", "extra_info", "status", "update_time",
		},
		JSONColumns: []string{"aliases", "keywords", "extra_info"},
		// Scopes the sync to the one-time NMPA catalog import, excluding
		// each deployment's own locally-generated status='proposed' rows
		// (server/api/doc-processing/product_names_resolve.go, extract-products.go).
		Filter: "source = 'cn_nmpa_medical_device_classification_catalog'",
	},
}

// ItemByID looks up a registered sync item by its stable id.
func ItemByID(id string) (TableSyncItem, bool) {
	for _, item := range Registry {
		if item.ID == id {
			return item, true
		}
	}
	return TableSyncItem{}, false
}

func (t TableSyncItem) isJSONColumn(col string) bool {
	for _, c := range t.JSONColumns {
		if c == col {
			return true
		}
	}
	return false
}

func (t TableSyncItem) isNaturalKeyColumn(col string) bool {
	for _, c := range t.NaturalKey {
		if c == col {
			return true
		}
	}
	return false
}
