package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// Relation types live in kb.product_relation_types (openspec change
// product-relation-types). productV2Relations in products_v2.go is only the seed
// and the fallback when no resolver is configured.

const defaultMaxProductRelationTypes = 50

// maxProductRelationTypes reads MAX_PRODUCT_RELATION_TYPES; a missing or invalid
// value means the default.
func maxProductRelationTypes() int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("MAX_PRODUCT_RELATION_TYPES"))); err == nil && n > 0 {
		return n
	}
	return defaultMaxProductRelationTypes
}

// ProductRelationUse is one distinct relation code seen in a model response.
type ProductRelationUse struct {
	Code        string
	Count       int64
	ProductName string
	Extra       map[string]any
}

// ProductRelationResolver maps relation codes to the value kb.products.relation_type
// stores. An unknown code is recorded as proposed and still resolves.
type ProductRelationResolver interface {
	ResolveProductRelations(ctx context.Context, recordID int64, uses []ProductRelationUse) (map[string]string, error)
}

type ProductRelationSQLStore struct {
	DB     *sql.DB
	Logger ApiTypes.JimoLogger
}

func (s ProductRelationSQLStore) ResolveProductRelations(
	ctx context.Context, recordID int64, uses []ProductRelationUse) (map[string]string, error) {
	out := make(map[string]string, len(uses))
	for _, use := range uses {
		// A model that writes the stored value ("storage_requirement") instead of the
		// code is accepted as that type.
		var value string
		err := s.DB.QueryRowContext(ctx, `
			UPDATE kb.product_relation_types SET seen_count = seen_count + $2
			WHERE code = $1 OR value = $1
			RETURNING value`, use.Code, use.Count).Scan(&value)
		if err == nil {
			out[use.Code] = value
			continue
		}
		if err != sql.ErrNoRows {
			return nil, fmt.Errorf("resolve relation type %q: %w (MID_20261118_001)", use.Code, err)
		}
		extra, _ := json.Marshal(use.Extra)
		var inserted bool
		err = s.DB.QueryRowContext(ctx, `
			INSERT INTO kb.product_relation_types
				(code, value, status, first_record_id, first_product_ref, seen_count, extra_info)
			VALUES ($1, $1, 'proposed', NULLIF($2, 0), $3, $4, $5::jsonb)
			ON CONFLICT (code) DO UPDATE
				SET seen_count = kb.product_relation_types.seen_count + EXCLUDED.seen_count
			RETURNING value, (xmax = 0)`,
			use.Code, recordID, use.ProductName, use.Count, string(extra)).Scan(&value, &inserted)
		if err != nil {
			return nil, fmt.Errorf("propose relation type %q: %w (MID_20261118_002)", use.Code, err)
		}
		out[use.Code] = value
		if inserted {
			s.warnProposed(ctx, recordID, use)
		}
	}
	return out, nil
}

func (s ProductRelationSQLStore) warnProposed(ctx context.Context, recordID int64, use ProductRelationUse) {
	if s.Logger == nil {
		return
	}
	s.Logger.Warn("new product relation type proposed; an admin should review it on System Admin > Database Maintenance > Product Relation Types",
		"code", use.Code, "record_id", recordID, "product", use.ProductName)
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM kb.product_relation_types`).Scan(&total); err != nil {
		return
	}
	if limit := maxProductRelationTypes(); total > limit {
		s.Logger.Warn("product relation type limit exceeded; the type was still accepted",
			"limit", limit, "count", total, "code", use.Code)
	}
}

// resolveProductRelations replaces each product's relation_type, which decode left as
// the seed value or the raw code, with the value the table resolves its relation_code
// to. relation_code is removed. With no resolver the decode value stands.
func (p *ProductsProcessor) resolveProductRelations(ctx context.Context, payload map[string]any) error {
	products, _ := payload["products"].([]any)
	uses := map[string]*ProductRelationUse{}
	var order []string
	for _, item := range products {
		row, _ := item.(map[string]any)
		code, _ := row["relation_code"].(string)
		if code == "" {
			continue
		}
		u := uses[code]
		if u == nil {
			u = &ProductRelationUse{Code: code, ProductName: asString(row["product_name"]),
				Extra: map[string]any{"evidence_lines": row["evidence_lines"], "summary": row["product_summary"]}}
			uses[code] = u
			order = append(order, code)
		}
		u.Count++
	}
	var values map[string]string
	if p.Relations != nil && len(order) > 0 {
		list := make([]ProductRelationUse, 0, len(order))
		for _, code := range order {
			list = append(list, *uses[code])
		}
		var err error
		if values, err = p.Relations.ResolveProductRelations(ctx, llmRecordIDFromContext(ctx), list); err != nil {
			return err
		}
	}
	for _, item := range products {
		row, _ := item.(map[string]any)
		code, _ := row["relation_code"].(string)
		if code == "" {
			continue
		}
		if v, ok := values[code]; ok {
			row["relation_type"] = v
		}
		delete(row, "relation_code")
	}
	return nil
}
