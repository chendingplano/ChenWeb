package productreviews

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
)

// ObjectName is the display name behind a kb.object_nodes object id, for
// resolving a scope node's grounded object_id (a foreign key) into something
// a reviewer can read.
type ObjectName struct {
	ObjectID string `json:"object_id"`
	Name     string `json:"name"`
	NameEN   string `json:"name_en"`
}

// FetchObjectNames resolves kb.object_nodes canonical names for a batch of
// object ids. Ids with no matching row are simply absent from the result.
func FetchObjectNames(ctx context.Context, db *sql.DB, objectIDs []string) ([]ObjectName, error) {
	objectIDs = dedupeStrings(objectIDs)
	if len(objectIDs) == 0 {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT object_id, COALESCE(canonical_name, ''), COALESCE(canonical_name_en, '')
		FROM kb.object_nodes WHERE object_id = ANY($1)`, pq.Array(objectIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ObjectName
	for rows.Next() {
		var o ObjectName
		if err := rows.Scan(&o.ObjectID, &o.Name, &o.NameEN); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
