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

	// References lists the columns that point at another compiled item's row
	// by surrogate id. Surrogate ids differ per deployment, so the source sends
	// the parent's NaturalKey values instead and the target resolves them to its
	// own id. Every referenced item must be a compiled Registry item, and must
	// be synced before this one.
	References []SyncReference
}

// SyncReference is one foreign-key column whose value travels as the parent
// row's natural key. The parent table must have a surrogate `id` column.
type SyncReference struct {
	// Column is one of Columns; it holds the surrogate id on each instance.
	Column string
	// ItemID is the id of the compiled Registry item whose table it points into.
	ItemID string
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
			"code_group", "child_code_group", "industry_code", "cpc", "entry_no", "entry_no_new",
		},
		JSONColumns: []string{"aliases", "keywords", "extra_info"},
		// Sync every approved row regardless of its source label, including
		// manually-added names. Locally-generated proposed rows remain excluded.
		Filter: "status = 'approved'",
	},
	// Holiday calendars. Each item's natural key is stable across instances;
	// surrogate ids never cross the wire (see SyncReference). Sync order:
	// calendar and holiday_info before holidays.
	{
		ID:         "calendar",
		Table:      "public.calendars",
		CursorCol:  "updated_at",
		NaturalKey: []string{"year", "country", "calendar_type"},
		Columns:    []string{"year", "country", "calendar_type", "created_at", "updated_at"},
	},
	{
		ID:         "holiday_info",
		Table:      "public.holiday_info",
		CursorCol:  "updated_at",
		NaturalKey: []string{"country", "calendar_type", "name"},
		Columns: []string{
			"country", "calendar_type", "name", "display_seqno", "description", "note",
			"created_at", "updated_at",
		},
	},
	{
		ID:         "holidays",
		Table:      "public.calendar_holidays",
		CursorCol:  "updated_at",
		NaturalKey: []string{"calendar_id", "holiday_date"},
		Columns: []string{
			"calendar_id", "holiday_info_id", "holiday_date", "day_kind", "created_at", "updated_at",
		},
		References: []SyncReference{
			{Column: "calendar_id", ItemID: "calendar"},
			{Column: "holiday_info_id", ItemID: "holiday_info"},
		},
	},
	// Singleton: CHECK (id = 1) pins the primary key to the same value on every
	// instance, so id is a stable natural key here, not a surrogate.
	{
		ID:         "calendar_default_country",
		Table:      "public.calendar_default_country",
		CursorCol:  "updated_at",
		NaturalKey: []string{"id"},
		Columns:    []string{"id", "country", "updated_at"},
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

func (t TableSyncItem) referenceFor(col string) (SyncReference, bool) {
	for _, ref := range t.References {
		if ref.Column == col {
			return ref, true
		}
	}
	return SyncReference{}, false
}

func (t TableSyncItem) isNaturalKeyColumn(col string) bool {
	for _, c := range t.NaturalKey {
		if c == col {
			return true
		}
	}
	return false
}
