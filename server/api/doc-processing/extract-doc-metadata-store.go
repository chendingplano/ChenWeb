package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type DocMetadataInputRecord struct {
	ID              int64
	ParserName      string
	ResultFilename  string
	StagingFilename string
	StatusRaw       string
	FileName        string
}

type DocMetadataUpdate struct {
	Title       string
	DocNo       string
	PublishDate string
	Authors     []string
	DocMetadata map[string]any
	StatusRaw   string
	ErrorMsg    *string
}

type DocMetadataStore interface {
	GetInputRecord(ctx context.Context, id int64) (DocMetadataInputRecord, error)
	UpdateInputMetadata(ctx context.Context, id int64, req DocMetadataUpdate) error
}

type DocMetadataSQLStore struct {
	DB *sql.DB
}

func (s DocMetadataSQLStore) GetInputRecord(ctx context.Context, id int64) (DocMetadataInputRecord, error) {
	const stmt = `
SELECT id,
       COALESCE(parser_name, ''),
       COALESCE(result_filename, ''),
       COALESCE(staging_filename, ''),
       COALESCE(status::text, '[]'),
       COALESCE(file_name, '')
FROM kb.inputs
WHERE id = $1`

	var rec DocMetadataInputRecord
	err := s.DB.QueryRowContext(ctx, stmt, id).Scan(
		&rec.ID,
		&rec.ParserName,
		&rec.ResultFilename,
		&rec.StagingFilename,
		&rec.StatusRaw,
		&rec.FileName,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DocMetadataInputRecord{}, sql.ErrNoRows
		}
		return DocMetadataInputRecord{}, err
	}
	return rec, nil
}

func (s DocMetadataSQLStore) UpdateInputMetadata(ctx context.Context, id int64, req DocMetadataUpdate) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	if strings.TrimSpace(req.StatusRaw) == "" {
		req.StatusRaw = "[]"
	}

	colTypes, err := s.loadColumnTypes(ctx)
	if err != nil {
		return fmt.Errorf("load kb.inputs column types: %w", err)
	}

	sets := make([]string, 0, 8)
	args := make([]any, 0, 12)

	add := func(clause string, arg any) {
		sets = append(sets, clause)
		args = append(args, arg)
	}

	if hasColumn(colTypes, "title") && strings.TrimSpace(req.Title) != "" {
		add(fmt.Sprintf("title = $%d", len(args)+1), strings.TrimSpace(req.Title))
	}
	if hasColumn(colTypes, "doc_no") && strings.TrimSpace(req.DocNo) != "" {
		add(fmt.Sprintf("doc_no = $%d", len(args)+1), strings.TrimSpace(req.DocNo))
	}
	if hasColumn(colTypes, "publish_date") && strings.TrimSpace(req.PublishDate) != "" {
		t := colTypes["publish_date"]
		switch {
		case strings.Contains(t, "date"):
			add(fmt.Sprintf("publish_date = NULLIF($%d, '')::date", len(args)+1), strings.TrimSpace(req.PublishDate))
		default:
			add(fmt.Sprintf("publish_date = $%d", len(args)+1), strings.TrimSpace(req.PublishDate))
		}
	}
	if hasColumn(colTypes, "authors") && len(req.Authors) > 0 {
		t := colTypes["authors"]
		switch {
		case strings.Contains(t, "[]") || strings.Contains(t, "array") || strings.HasPrefix(t, "_"):
			add(fmt.Sprintf("authors = $%d::text[]", len(args)+1), pgTextArrayLiteral(req.Authors))
		case strings.Contains(t, "json"):
			bs, err := json.Marshal(req.Authors)
			if err != nil {
				return fmt.Errorf("marshal authors json: %w", err)
			}
			add(fmt.Sprintf("authors = $%d::jsonb", len(args)+1), string(bs))
		default:
			add(fmt.Sprintf("authors = $%d", len(args)+1), strings.Join(req.Authors, ", "))
		}
	}
	if hasColumn(colTypes, "doc_metadata") && req.DocMetadata != nil {
		bs, err := json.Marshal(req.DocMetadata)
		if err != nil {
			return fmt.Errorf("marshal doc_metadata: %w", err)
		}
		t := colTypes["doc_metadata"]
		if strings.Contains(t, "json") {
			add(fmt.Sprintf("doc_metadata = $%d::jsonb", len(args)+1), string(bs))
		} else {
			add(fmt.Sprintf("doc_metadata = $%d", len(args)+1), string(bs))
		}
	}

	add(fmt.Sprintf("status = $%d::jsonb", len(args)+1), req.StatusRaw)
	add(fmt.Sprintf("error_msg = $%d", len(args)+1), req.ErrorMsg)
	sets = append(sets, "modify_time = NOW()")

	args = append(args, id)
	stmt := fmt.Sprintf("UPDATE kb.inputs SET %s WHERE id = $%d", strings.Join(sets, ",\n    "), len(args))

	_, err = s.DB.ExecContext(ctx, stmt, args...)
	return err
}

func (s DocMetadataSQLStore) loadColumnTypes(ctx context.Context) (map[string]string, error) {
	const q = `
SELECT column_name,
       data_type,
       COALESCE(udt_name, '')
FROM information_schema.columns
WHERE table_schema = 'kb'
  AND table_name = 'inputs'
  AND column_name IN ('title', 'doc_no', 'publish_date', 'authors', 'doc_metadata')`
	rows, err := s.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var name, dataType, udt string
		if err := rows.Scan(&name, &dataType, &udt); err != nil {
			return nil, err
		}
		out[name] = normalizeDBType(dataType, udt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func normalizeDBType(dataType string, udt string) string {
	d := strings.ToLower(strings.TrimSpace(dataType))
	u := strings.ToLower(strings.TrimSpace(udt))
	if d == "array" {
		return u
	}
	if u != "" {
		return d + ":" + u
	}
	return d
}

func pgTextArrayLiteral(in []string) string {
	vals := make([]string, 0, len(in))
	for _, v := range in {
		t := strings.TrimSpace(v)
		if t == "" {
			continue
		}
		t = strings.ReplaceAll(t, `\\`, `\\\\`)
		t = strings.ReplaceAll(t, `"`, `\\"`)
		vals = append(vals, `"`+t+`"`)
	}
	if len(vals) == 0 {
		return "{}"
	}
	return "{" + strings.Join(vals, ",") + "}"
}

func hasColumn(cols map[string]string, name string) bool {
	_, ok := cols[name]
	return ok
}
