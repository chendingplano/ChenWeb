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
	// Columns is the full set of columns to transfer, excluding surrogate
	// primary-key columns unless they are also part of the explicit NaturalKey.
	// Must include every column in NaturalKey and CursorCol.
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

	// External terminology resources (QUDT, UCUM, BIPM SIRP, Wikidata). Only
	// the dev machine downloads and imports them; deployments pull the same
	// rows, so every machine uses exactly the data the releasing machine does.
	// Sync order: keyword_sources first, then everything that references a
	// source/release, in the order listed (the catalog child tables reference
	// keyword_catalog_entries).
	//
	// These tables are immutable, so created_at is an insert-time cursor. Every
	// release travels except a rejected one: an unreviewed release is used as
	// dictionary data on the releasing machine, so it is used the same way on a
	// deployment. (Only an approved release can be an identity authority.)
	// Rows are keyed by the same (source, release, ...) identity as the
	// primary key; no surrogate id exists to leave out.
	{
		ID:         "keyword_sources",
		Table:      "kb.keyword_sources",
		CursorCol:  "created_at",
		NaturalKey: []string{"source", "release"},
		Columns: []string{
			"source", "release", "license", "retrieved_at", "notes", "provider_id",
			"source_subset", "content_checksum", "license_review_status", "authority_role",
			"authoritative_relations", "allowed_scopes", "languages", "adapter_version",
			"provenance_locator", "approved_by", "approved_at", "identity_authority", "created_at",
		},
		Filter: "license_review_status <> 'rejected'",
	},
	{
		ID:         "keyword_source_artifacts",
		Table:      "kb.keyword_source_artifacts",
		CursorCol:  "created_at",
		NaturalKey: []string{"source", "release", "artifact_id"},
		Columns: []string{
			"source", "release", "artifact_id", "content_checksum", "media_type",
			"provenance_locator", "payload", "created_at",
		},
		JSONColumns: []string{"payload"},
		Filter:      usableKeywordSourceFilter,
	},
	{
		ID:         "keyword_catalog_entries",
		Table:      "kb.keyword_catalog_entries",
		CursorCol:  "created_at",
		NaturalKey: []string{"source", "release", "external_id"},
		Columns: []string{
			"source", "release", "external_id", "entry_status", "provenance_locator",
			"native_payload", "created_at",
		},
		JSONColumns: []string{"native_payload"},
		Filter:      usableKeywordSourceFilter,
	},
	{
		ID:         "keyword_catalog_labels",
		Table:      "kb.keyword_catalog_labels",
		CursorCol:  "created_at",
		NaturalKey: []string{"source", "release", "external_id", "language", "label_role", "label"},
		Columns: []string{
			"source", "release", "external_id", "language", "label_role", "label",
			"provenance_locator", "native_payload", "created_at",
		},
		JSONColumns: []string{"native_payload"},
		Filter:      usableKeywordSourceFilter,
	},
	{
		ID:         "keyword_catalog_relations",
		Table:      "kb.keyword_catalog_relations",
		CursorCol:  "created_at",
		NaturalKey: []string{"source", "release", "subject_external_id", "relation", "object_source", "object_release", "object_external_id"},
		Columns: []string{
			"source", "release", "subject_external_id", "relation", "object_source",
			"object_release", "object_external_id", "provenance_locator", "native_payload", "created_at",
		},
		JSONColumns: []string{"native_payload"},
		Filter:      usableKeywordSourceFilter,
	},
	{
		ID:         "keyword_catalog_negative_decisions",
		Table:      "kb.keyword_catalog_negative_decisions",
		CursorCol:  "created_at",
		NaturalKey: []string{"source", "release", "subject_external_id", "object_source", "object_release", "object_external_id", "relation"},
		Columns: []string{
			"source", "release", "subject_external_id", "object_source", "object_release",
			"object_external_id", "relation", "reason", "provenance_locator", "native_payload", "created_at",
		},
		JSONColumns: []string{"native_payload"},
		Filter:      usableKeywordSourceFilter,
	},
	{
		ID:         "keyword_ucum_codes",
		Table:      "kb.keyword_ucum_codes",
		CursorCol:  "created_at",
		NaturalKey: []string{"source", "release", "code"},
		Columns: []string{
			"source", "release", "code", "print_symbol", "dimension", "provenance_locator",
			"native_payload", "created_at",
		},
		JSONColumns: []string{"native_payload"},
		Filter:      usableKeywordSourceFilter,
	},

	// The keyword dictionary, mirrored as it is on the releasing machine: the
	// concepts and surfaces, a surface's derived lookup keys, the normalization
	// rewrite rules, and the identity mappings from external sources. Every row is
	// synced, whatever created it; concept_id, surface_id and rule_id are stable
	// text ids (never surrogates). The sync only adds and updates, so a row that
	// exists only on a deployment is left alone. Sync order: concepts, surfaces,
	// then the tables that point at them; external_ids and surface_evidence also
	// need keyword_sources.
	{
		ID:         "keyword_concepts",
		Table:      "kb.keyword_concepts",
		CursorCol:  "modify_time",
		NaturalKey: []string{"concept_id"},
		Columns: []string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into",
			"gloss_source", "create_time", "modify_time",
		},
	},
	{
		ID:         "keyword_surfaces",
		Table:      "kb.keyword_surfaces",
		CursorCol:  "modify_time",
		NaturalKey: []string{"surface_id"},
		Columns: []string{
			"surface_id", "concept_id", "surface", "norm_key", "norm_version", "label_role",
			"alias_type", "lang", "scope", "confidence", "provenance", "locked", "evidence",
			"create_time", "modify_time", "origin_concept",
		},
	},
	// The application never edits a key row: it replaces all keys of a surface, so
	// created_at is a complete cursor.
	{
		ID:         "keyword_surface_keys",
		Table:      "kb.keyword_surface_keys",
		CursorCol:  "created_at",
		NaturalKey: []string{"surface_id", "key_kind"},
		Columns:    []string{"surface_id", "key_kind", "key_value", "norm_version", "created_at"},
	},
	{
		ID:         "keyword_rewrite_rules",
		Table:      "kb.keyword_rewrite_rules",
		CursorCol:  "modify_time",
		NaturalKey: []string{"rule_id"},
		Columns: []string{
			"rule_id", "pattern", "replacement", "scope", "enabled", "provenance",
			"create_time", "modify_time",
		},
	},
	{
		ID:         "keyword_external_ids",
		Table:      "kb.keyword_external_ids",
		CursorCol:  "modify_time",
		NaturalKey: []string{"source", "external_id", "release"},
		Columns:    []string{"source", "external_id", "release", "concept_id", "modify_time"},
		Filter:     usableKeywordSourceFilter,
	},
	{
		ID:         "keyword_surface_evidence",
		Table:      "kb.keyword_surface_evidence",
		CursorCol:  "modify_time",
		NaturalKey: []string{"surface_id", "source", "external_id", "release"},
		Columns: []string{
			"surface_id", "source", "external_id", "evidence", "confidence", "release", "modify_time",
		},
		Filter: usableKeywordSourceFilter,
	},

	// The governed `quantity` ontology module that the QUDT approve flow
	// writes from the downloaded TTL. The TTL is a transient download, so a
	// deployment cannot re-derive these rows; it must receive them. Sync order:
	// module, release, then everything that points at the release (terms,
	// labels, mappings, active pointer). kb.ontology_term_headers and
	// kb.ontology_term_revisions are filled by a trigger when a term row is
	// inserted, so they are not synced.
	//
	// Surrogate ids (id, released_in_release_id, release_id) never cross the
	// wire: releases are addressed by (module_id, version). source_candidate_id
	// is a local candidate-queue id and is left out. superseded_by_release_id
	// is left out too: it points at a newer release that is not yet on the
	// target when the older row is applied, so it stays NULL on a deployment.
	{
		ID:         "ontology_quantity_module",
		Table:      "kb.ontology_modules",
		CursorCol:  "modify_time",
		NaturalKey: []string{"module_id"},
		Columns: []string{
			"module_id", "title", "owner", "description", "depends_on", "status",
			"create_time", "create_by", "modify_time", "modify_by",
		},
		Filter: "module_id = 'quantity'",
	},
	{
		ID:         "ontology_quantity_releases",
		Table:      "kb.ontology_module_releases",
		CursorCol:  "modify_time",
		NaturalKey: []string{"module_id", "version"},
		Columns: []string{
			"module_id", "version", "title", "payload", "content_checksum",
			"dependency_releases", "released_by", "released_at", "create_time", "modify_time",
		},
		JSONColumns: []string{"payload", "dependency_releases"},
		Filter:      "module_id = 'quantity'",
	},
	{
		ID:         "ontology_quantity_terms",
		Table:      "kb.ontology_terms",
		CursorCol:  "modify_time",
		NaturalKey: []string{"term_id", "version"},
		Columns: []string{
			"term_id", "version", "term_kind", "module_id", "status", "definition", "scope",
			"create_time", "create_by", "modify_time", "modify_by", "released_in_release_id", "properties",
		},
		JSONColumns: []string{"properties"},
		Filter:      "module_id = 'quantity'",
		References:  []SyncReference{{Column: "released_in_release_id", ItemID: "ontology_quantity_releases"}},
	},
	{
		ID:         "ontology_quantity_term_labels",
		Table:      "kb.ontology_term_labels",
		CursorCol:  "modify_time",
		NaturalKey: []string{"term_id", "version", "lang", "label_role", "label"},
		Columns: []string{
			"term_id", "version", "label", "lang", "label_role", "status",
			"create_time", "create_by", "modify_time", "modify_by", "released_in_release_id",
		},
		Filter:     "term_id IN (SELECT term_id FROM kb.ontology_terms WHERE module_id = 'quantity')",
		References: []SyncReference{{Column: "released_in_release_id", ItemID: "ontology_quantity_releases"}},
	},
	{
		ID:         "ontology_quantity_mappings",
		Table:      "kb.ontology_mappings",
		CursorCol:  "modify_time",
		NaturalKey: []string{"mapping_id", "version"},
		Columns: []string{
			"mapping_id", "version", "from_term_id", "to_term_id", "to_iri", "relation",
			"evidence", "approval_status", "module_id", "status", "create_time", "create_by",
			"modify_time", "modify_by", "released_in_release_id",
		},
		JSONColumns: []string{"evidence"},
		Filter:      "module_id = 'quantity'",
		References:  []SyncReference{{Column: "released_in_release_id", ItemID: "ontology_quantity_releases"}},
	},
	// The pointer is a history (a release can be activated again later), so an
	// activation is keyed by when it happened too. Replacing a release sets
	// deactivated_at on the old row (modify_time = transaction time) and then
	// inserts the new row (modify_time = clock_timestamp(), later), so the pull
	// delivers the deactivation first and the one-active-release-per-module
	// index never rejects the new row.
	{
		ID:         "ontology_quantity_active_releases",
		Table:      "kb.ontology_active_releases",
		CursorCol:  "modify_time",
		NaturalKey: []string{"module_id", "release_id", "activated_at"},
		Columns: []string{
			"module_id", "release_id", "activated_at", "activated_by", "deactivated_at",
			"deactivated_by", "modify_time",
		},
		Filter:     "module_id = 'quantity'",
		References: []SyncReference{{Column: "release_id", ItemID: "ontology_quantity_releases"}},
	},
}

// usableKeywordSourceFilter scopes a keyword catalog table to rows whose
// (source, release) is a release in kb.keyword_sources that has not been rejected.
const usableKeywordSourceFilter = "(source, release) IN (SELECT source, release FROM kb.keyword_sources WHERE license_review_status <> 'rejected')"

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
