package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// Admin review of kb.product_relation_types (openspec change product-relation-types).
// The extractor inserts unknown relation codes as 'proposed'; here an admin approves them
// with descriptions and a weight, edits approved ones, or removes unused proposals.

const defaultProductRelationWeight = 0.5

var (
	errRelationTypeNotFound = errors.New("product relation type not found")
	errRelationTypeInUse    = errors.New("product relation type is in use")
	errRelationTypeApproved = errors.New("approved product relation type cannot be deleted")
)

type productRelationTypeRow struct {
	Code            string          `json:"code"`
	Value           string          `json:"value"`
	Status          string          `json:"status"`
	DescriptionEN   string          `json:"description_en"`
	DescriptionZH   string          `json:"description_zh"`
	Weight          float64         `json:"weight"`
	FirstRecordID   *int64          `json:"first_record_id"`
	FirstProductRef string          `json:"first_product_ref"`
	SeenCount       int64           `json:"seen_count"`
	ProductCount    int64           `json:"product_count"`
	ExtraInfo       json.RawMessage `json:"extra_info"`
	CreatedAt       time.Time       `json:"created_at"`
	ApprovedAt      *time.Time      `json:"approved_at"`
	ApprovedBy      string          `json:"approved_by"`
}

type productRelationTypeInput struct {
	DescriptionEN string   `json:"description_en"`
	DescriptionZH string   `json:"description_zh"`
	Weight        *float64 `json:"weight"`
}

// validate trims the descriptions and applies the default weight. A blank description or a
// weight outside 0..1 is refused.
func (in productRelationTypeInput) validate() (productRelationTypeInput, float64, string) {
	in.DescriptionEN = strings.TrimSpace(in.DescriptionEN)
	in.DescriptionZH = strings.TrimSpace(in.DescriptionZH)
	if in.DescriptionEN == "" || in.DescriptionZH == "" {
		return in, 0, "English and Chinese descriptions are both required"
	}
	weight := defaultProductRelationWeight
	if in.Weight != nil {
		weight = *in.Weight
	}
	if weight < 0 || weight > 1 {
		return in, 0, "weight must be between 0 and 1"
	}
	return in, weight, ""
}

type productRelationTypeStore interface {
	List(context.Context) ([]productRelationTypeRow, error)
	Approve(ctx context.Context, code, actor string, in productRelationTypeInput, weight float64) (productRelationTypeRow, error)
	Update(ctx context.Context, code string, in productRelationTypeInput, weight float64) (productRelationTypeRow, error)
	Delete(ctx context.Context, code string) error
}

type productRelationTypeSQLStore struct{ DB *sql.DB }

const productRelationTypeColumns = `t.code, t.value, t.status, t.description_en, t.description_zh,
	t.weight::float8, t.first_record_id, t.first_product_ref, t.seen_count,
	(SELECT count(*) FROM kb.products p WHERE p.relation_type = t.value),
	t.extra_info, t.created_at, t.approved_at, COALESCE(t.approved_by, '')`

func scanProductRelationType(row interface{ Scan(...any) error }) (productRelationTypeRow, error) {
	var r productRelationTypeRow
	var first sql.NullInt64
	var approved sql.NullTime
	err := row.Scan(&r.Code, &r.Value, &r.Status, &r.DescriptionEN, &r.DescriptionZH, &r.Weight,
		&first, &r.FirstProductRef, &r.SeenCount, &r.ProductCount, &r.ExtraInfo, &r.CreatedAt, &approved, &r.ApprovedBy)
	if first.Valid {
		r.FirstRecordID = &first.Int64
	}
	if approved.Valid {
		r.ApprovedAt = &approved.Time
	}
	return r, err
}

func (s productRelationTypeSQLStore) List(ctx context.Context) ([]productRelationTypeRow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+productRelationTypeColumns+
		` FROM kb.product_relation_types t ORDER BY (t.status = 'proposed') DESC, t.code`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []productRelationTypeRow{}
	for rows.Next() {
		r, err := scanProductRelationType(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s productRelationTypeSQLStore) write(ctx context.Context, query string, args ...any) (productRelationTypeRow, error) {
	r, err := scanProductRelationType(s.DB.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return r, errRelationTypeNotFound
	}
	return r, err
}

func (s productRelationTypeSQLStore) Approve(ctx context.Context, code, actor string, in productRelationTypeInput, weight float64) (productRelationTypeRow, error) {
	return s.write(ctx, `
		WITH u AS (
			UPDATE kb.product_relation_types
			   SET status = 'approved', description_en = $2, description_zh = $3, weight = $4,
			       approved_at = NOW(), approved_by = $5
			 WHERE code = $1 RETURNING *)
		SELECT `+productRelationTypeColumns+` FROM u t`,
		code, in.DescriptionEN, in.DescriptionZH, weight, actor)
}

func (s productRelationTypeSQLStore) Update(ctx context.Context, code string, in productRelationTypeInput, weight float64) (productRelationTypeRow, error) {
	return s.write(ctx, `
		WITH u AS (
			UPDATE kb.product_relation_types
			   SET description_en = $2, description_zh = $3, weight = $4
			 WHERE code = $1 AND status = 'approved' RETURNING *)
		SELECT `+productRelationTypeColumns+` FROM u t`,
		code, in.DescriptionEN, in.DescriptionZH, weight)
}

func (s productRelationTypeSQLStore) Delete(ctx context.Context, code string) error {
	var status, value string
	err := s.DB.QueryRowContext(ctx,
		`SELECT status, value FROM kb.product_relation_types WHERE code = $1`, code).Scan(&status, &value)
	if errors.Is(err, sql.ErrNoRows) {
		return errRelationTypeNotFound
	}
	if err != nil {
		return err
	}
	if status != "proposed" {
		return errRelationTypeApproved
	}
	var used int64
	if err := s.DB.QueryRowContext(ctx,
		`SELECT count(*) FROM kb.products WHERE relation_type = $1`, value).Scan(&used); err != nil {
		return err
	}
	if used > 0 {
		return errRelationTypeInUse
	}
	_, err = s.DB.ExecContext(ctx,
		`DELETE FROM kb.product_relation_types WHERE code = $1 AND status = 'proposed'`, code)
	return err
}

func ListProductRelationTypes(c echo.Context) error {
	return listProductRelationTypes(c, productRelationTypeSQLStore{DB: ApiTypes.ProjectDBHandle})
}

func ApproveProductRelationType(c echo.Context) error {
	return writeProductRelationType(c, productRelationTypeSQLStore{DB: ApiTypes.ProjectDBHandle}, true)
}

func UpdateProductRelationType(c echo.Context) error {
	return writeProductRelationType(c, productRelationTypeSQLStore{DB: ApiTypes.ProjectDBHandle}, false)
}

func DeleteProductRelationType(c echo.Context) error {
	return deleteProductRelationType(c, productRelationTypeSQLStore{DB: ApiTypes.ProjectDBHandle})
}

func listProductRelationTypes(c echo.Context, store productRelationTypeStore) error {
	rc := EchoFactory.NewFromEcho(c, "20261118-601")
	defer rc.Close()
	if _, ok := objectTypeReviewAdmin(c, rc); !ok {
		return nil
	}
	rows, err := store.List(c.Request().Context())
	if err != nil {
		rc.GetLogger().Error("product relation types: list failed", "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list product relation types"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "rows": rows})
}

func writeProductRelationType(c echo.Context, store productRelationTypeStore, approve bool) error {
	rc := EchoFactory.NewFromEcho(c, "20261118-602")
	defer rc.Close()
	user, ok := objectTypeReviewAdmin(c, rc)
	if !ok {
		return nil
	}
	code := strings.ToLower(strings.TrimSpace(c.Param("code")))
	var in productRelationTypeInput
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	in, weight, problem := in.validate()
	if problem != "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: problem})
	}
	var row productRelationTypeRow
	var err error
	action := "updated"
	if approve {
		action = "approved"
		row, err = store.Approve(c.Request().Context(), code, user.UserId, in, weight)
	} else {
		row, err = store.Update(c.Request().Context(), code, in, weight)
	}
	if errors.Is(err, errRelationTypeNotFound) {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "product relation type not found"})
	}
	if err != nil {
		rc.GetLogger().Error("product relation types: write failed", "code", code, "action", action, "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to save product relation type"})
	}
	rc.GetLogger().Info("product relation types: "+action, "code", code, "weight", weight, "actor", user.UserId)
	return c.JSON(http.StatusOK, map[string]any{"status": true, "row": row})
}

func deleteProductRelationType(c echo.Context, store productRelationTypeStore) error {
	rc := EchoFactory.NewFromEcho(c, "20261118-603")
	defer rc.Close()
	user, ok := objectTypeReviewAdmin(c, rc)
	if !ok {
		return nil
	}
	code := strings.ToLower(strings.TrimSpace(c.Param("code")))
	err := store.Delete(c.Request().Context(), code)
	switch {
	case errors.Is(err, errRelationTypeNotFound):
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "product relation type not found"})
	case errors.Is(err, errRelationTypeInUse), errors.Is(err, errRelationTypeApproved):
		return c.JSON(http.StatusConflict, errorResponse{Status: false, ErrorMsg: err.Error()})
	case err != nil:
		rc.GetLogger().Error("product relation types: delete failed", "code", code, "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete product relation type"})
	}
	rc.GetLogger().Info("product relation types: deleted", "code", code, "actor", user.UserId)
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
