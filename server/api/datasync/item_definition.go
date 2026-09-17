package datasync

// itemDefinition is the wire shape of one sync item's shape: what the
// discovery endpoint (pull_handler.go's HandleListSourceItems) advertises,
// and what a target decodes and write-through-caches (client.go's
// fetchSourceItemDefinitions, admin_handler.go's HandleListSyncItems). See
// openspec/changes/configurable-data-sync-items/design.md Decision 4.
type itemDefinition struct {
	ID                   string   `json:"id"`
	Kind                 string   `json:"kind"`
	Table                string   `json:"table"`
	CursorCol            string   `json:"cursor_col"`
	NaturalKey           []string `json:"natural_key"`
	Columns              []string `json:"columns"`
	JSONColumns          []string `json:"json_columns"`
	Filter               string   `json:"filter,omitempty"`
	FileColumn           string   `json:"file_column,omitempty"`
	FileDirEnv           string   `json:"file_dir_env,omitempty"`
	FileDirDefaultSubdir string   `json:"file_dir_default_subdir,omitempty"`
}

func toItemDefinition(item TableSyncItem) itemDefinition {
	return itemDefinition{
		ID:                   item.ID,
		Kind:                 kindOrDefault(item.Kind),
		Table:                item.Table,
		CursorCol:            item.CursorCol,
		NaturalKey:           item.NaturalKey,
		Columns:              item.Columns,
		JSONColumns:          item.JSONColumns,
		Filter:               item.Filter,
		FileColumn:           item.FileColumn,
		FileDirEnv:           item.FileDirEnv,
		FileDirDefaultSubdir: item.FileDirDefaultSubdir,
	}
}

func fromItemDefinition(def itemDefinition) TableSyncItem {
	return TableSyncItem{
		ID:                   def.ID,
		Kind:                 SyncKind(def.Kind),
		Table:                def.Table,
		CursorCol:            def.CursorCol,
		NaturalKey:           def.NaturalKey,
		Columns:              def.Columns,
		JSONColumns:          def.JSONColumns,
		Filter:               def.Filter,
		FileColumn:           def.FileColumn,
		FileDirEnv:           def.FileDirEnv,
		FileDirDefaultSubdir: def.FileDirDefaultSubdir,
	}
}
