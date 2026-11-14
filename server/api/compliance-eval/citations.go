package complianceeval

import (
	"context"
	"database/sql"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/lib/pq"
)

// RecordName is what identifies a knowledge-base record as a cited document.
type RecordName struct {
	RecordID                                int64
	DocNo, Title, FileName, StagingFilename string
}

// CitationIndex finds the records that hold a document an activity cites (design D11). A citation
// with an identifier ("GB/T 19095-2019") matches by the extract_references identity key, which has
// no edition, so every edition the knowledge base holds is found; a citation without one (a law
// cited by its title) matches a record title exactly, ignoring case, spaces, brackets and quotes.
type CitationIndex struct {
	byKey, byTitle map[string][]int64
}

// titleNoise is removed from titles and title citations before they are compared.
var titleNoise = strings.NewReplacer("《", "", "》", "", "〈", "", "〉", "", "“", "", "”", "", "\"", "",
	"「", "", "」", "", "'", "", "‘", "", "’", "")

func titleKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(titleNoise.Replace(s))), "")
}

// NewCitationIndex indexes the records a pool may use (the snapshot's records, the document and its
// copies left out).
func NewCitationIndex(recs []RecordName) CitationIndex {
	ix := CitationIndex{byKey: map[string][]int64{}, byTitle: map[string][]int64{}}
	for _, r := range recs {
		if k, ok := docprocessing.RefOwnIdentityKey(r.DocNo, r.Title, r.FileName, r.StagingFilename); ok {
			ix.byKey[k] = append(ix.byKey[k], r.RecordID)
		}
		if t := titleKey(r.Title); t != "" {
			ix.byTitle[t] = append(ix.byTitle[t], r.RecordID)
		}
	}
	return ix
}

// Resolve returns the records holding the cited document, or none.
func (ix CitationIndex) Resolve(citation string) []int64 {
	if k, ok := docprocessing.RefCitationIdentityKey(citation); ok {
		return ix.byKey[k]
	}
	return ix.byTitle[titleKey(citation)]
}

// LoadRecordNames reads the identifying names of the given records.
func LoadRecordNames(ctx context.Context, db *sql.DB, ids []int64) ([]RecordName, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, COALESCE(doc_no, ''), COALESCE(title, ''), COALESCE(file_name, ''),
		COALESCE(staging_filename, '') FROM kb.inputs WHERE id = ANY($1) ORDER BY id`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecordName
	for rows.Next() {
		var r RecordName
		if err := rows.Scan(&r.RecordID, &r.DocNo, &r.Title, &r.FileName, &r.StagingFilename); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
