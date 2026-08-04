package keywords

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Mention is one keyword mention observed in document text. Append-only —
// the observe-mode collector writes mentions, never updates or deletes them.
type Mention struct {
	MentionID   int64     `json:"mention_id"`
	ArtifactRef *string   `json:"artifact_ref"`
	ChunkRef    *string   `json:"chunk_ref"`
	ContextText *string   `json:"context_text"`
	KsID        *string   `json:"ks_id"`
	CreateTime  time.Time `json:"create_time"`
}

// MentionStore persists keyword mention rows (append-only).
type MentionStore struct {
	DB DBX
}

const mentionColumns = `mention_id, artifact_ref, chunk_ref, context_text, ks_id, create_time`

const mentionFrom = `FROM kb.keyword_mentions`

func scanMention(scan func(dest ...any) error) (Mention, error) {
	var (
		m           Mention
		artifactRef sql.NullString
		chunkRef    sql.NullString
		contextText sql.NullString
		ksID        sql.NullString
	)
	if err := scan(&m.MentionID, &artifactRef, &chunkRef, &contextText, &ksID, &m.CreateTime); err != nil {
		return Mention{}, err
	}
	if artifactRef.Valid {
		m.ArtifactRef = &artifactRef.String
	}
	if chunkRef.Valid {
		m.ChunkRef = &chunkRef.String
	}
	if contextText.Valid {
		m.ContextText = &contextText.String
	}
	if ksID.Valid {
		m.KsID = &ksID.String
	}
	return m, nil
}

// InsertMention inserts a single mention and returns its id.
func (s MentionStore) InsertMention(ctx context.Context, m Mention) (int64, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO kb.keyword_mentions (artifact_ref, chunk_ref, context_text, ks_id)
		VALUES ($1, $2, $3, $4)
		RETURNING mention_id`,
		nullableString(func() string {
			if m.ArtifactRef != nil {
				return *m.ArtifactRef
			}
			return ""
		}()), nullableString(func() string {
			if m.ChunkRef != nil {
				return *m.ChunkRef
			}
			return ""
		}()), nullableString(func() string {
			if m.ContextText != nil {
				return *m.ContextText
			}
			return ""
		}()), nullableString(func() string {
			if m.KsID != nil {
				return *m.KsID
			}
			return ""
		}()))
	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, fmt.Errorf("insert mention: %w", err)
	}
	return id, nil
}

// InsertMentions inserts multiple mentions in batch.
func (s MentionStore) InsertMentions(ctx context.Context, mentions []Mention) ([]int64, error) {
	ids := make([]int64, 0, len(mentions))
	for _, m := range mentions {
		id, err := s.InsertMention(ctx, m)
		if err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ListMentions returns mentions, optionally filtered by ks_id.
func (s MentionStore) ListMentions(ctx context.Context, ksID string, limit int) ([]Mention, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if limit <= 0 {
		limit = 50
	}
	if ksID == "" {
		rows, err = s.DB.QueryContext(ctx, `
			SELECT `+mentionColumns+`
			`+mentionFrom+`
			ORDER BY mention_id DESC
			LIMIT $1`, limit)
	} else {
		rows, err = s.DB.QueryContext(ctx, `
			SELECT `+mentionColumns+`
			`+mentionFrom+`
			WHERE ks_id = $1
			ORDER BY mention_id DESC
			LIMIT $2`, ksID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Mention
	for rows.Next() {
		m, err := scanMention(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
