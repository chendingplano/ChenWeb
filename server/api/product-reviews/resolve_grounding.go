package productreviews

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

type ResolveGroundingInput struct {
	ObjectID    string   `json:"object_id"`
	Create      bool     `json:"create"`
	Name        string   `json:"name"`
	NameEN      string   `json:"name_en"`
	NameZH      string   `json:"name_zh"`
	Aliases     []string `json:"aliases"`
	Description string   `json:"description"`
}

var errGroundingConflict = errors.New("node is no longer an ungrounded module or part")
var errGroundingObject = errors.New("choose an existing object that is not rejected, merged or awaiting disambiguation")
var errGroundingDuplicate = errors.New("an object with this name or alias already exists; search and link the existing object")
var errGroundingInput = errors.New("choose an existing object or provide a name to create one")

// ResolveGrounding locks the profile node and atomically creates/links an object.
// Grounding enriches the existing scope identity; saved run snapshots are unchanged.
func (s Store) ResolveGrounding(ctx context.Context, profileID, nodeID int64, in ResolveGroundingInput) (string, error) {
	in.ObjectID, in.Name = strings.TrimSpace(in.ObjectID), strings.TrimSpace(in.Name)
	if (in.Create && (in.Name == "" || in.ObjectID != "")) || (!in.Create && in.ObjectID == "") {
		return "", errGroundingInput
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var kind, grounding, status string
	err = tx.QueryRowContext(ctx, `SELECT node_kind, COALESCE(grounding,''), status
		FROM kb.product_profile_nodes WHERE profile_id = $1 AND id = $2 FOR UPDATE`, profileID, nodeID).Scan(&kind, &grounding, &status)
	if err == sql.ErrNoRows {
		return "", ErrNodeNotFound
	}
	if err != nil {
		return "", err
	}
	if (kind != KindModule && kind != KindPart) || status == StatusRejected || (grounding != "" && grounding != GroundingUngrounded) {
		return "", errGroundingConflict
	}
	objectID, reconcile := in.ObjectID, ""
	if in.Create {
		names := dedupeStrings(append([]string{in.Name, in.NameEN, in.NameZH}, in.Aliases...))
		normalized := make([]string, 0, len(names))
		for _, name := range names {
			if n := normalizeName(name); n != "" {
				normalized = append(normalized, n)
			}
		}
		// Lock all names in stable order, including aliases shared by concurrent requests.
		normalized = dedupeStrings(normalized)
		sort.Strings(normalized)
		for _, name := range normalized {
			if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, name); err != nil {
				return "", err
			}
		}
		var exists bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM kb.object_nodes
			WHERE reconcile_status NOT IN ('rejected','merged') AND
			(normalized_names ?| $1 OR lower(canonical_name) = ANY($1) OR lower(canonical_name_en) = ANY($1) OR lower(canonical_name_zh) = ANY($1)))`, pq.Array(normalized)).Scan(&exists)
		if err != nil {
			return "", err
		}
		if exists {
			return "", errGroundingDuplicate
		}
		node, err := (docprocessing.ObjectNodeSQLStore{DB: s.DB}).CreateNodeInTx(ctx, tx, docprocessing.ArtifactObject{
			ObjectName: in.Name, ObjectNameEn: strings.TrimSpace(in.NameEN), ObjectNameZh: strings.TrimSpace(in.NameZH),
			Aliases: dedupeStrings(in.Aliases), NormalizedNames: normalized, Description: strings.TrimSpace(in.Description),
			ObjectType: kind, ArtifactType: "product_profile", ArtifactID: in.Name,
		})
		if err != nil {
			return "", err
		}
		objectID = node.ObjectID
	}
	// Recheck after creation too: ON CONFLICT may have found a previously rejected object.
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(reconcile_status,'') FROM kb.object_nodes
		WHERE object_id = $1 FOR SHARE`, objectID).Scan(&reconcile)
	if err == sql.ErrNoRows {
		return "", errGroundingObject
	}
	if err != nil {
		return "", err
	}
	switch reconcile {
	case "rejected", "merged", "ambiguous", "pending_review":
		return "", errGroundingObject
	}
	_, err = tx.ExecContext(ctx, `UPDATE kb.product_profile_nodes SET grounding = 'object_node', object_id = $3,
		concept_id = NULL, term_id = NULL, reconcile_status = $4, updated_at = NOW()
		WHERE profile_id = $1 AND id = $2`, profileID, nodeID, objectID, reconcile)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return objectID, nil
}

func ResolveProductProfileGrounding(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "20261124-631")
	defer rc.Close()
	profileID, err := idParam(c, "id")
	if err != nil || profileID <= 0 {
		return c.NoContent(http.StatusBadRequest)
	}
	nodeID, err := idParam(c, "node_id")
	if err != nil || nodeID <= 0 {
		return c.NoContent(http.StatusBadRequest)
	}
	var in ResolveGroundingInput
	if err := c.Bind(&in); err != nil {
		return c.NoContent(http.StatusBadRequest)
	}
	objectID, err := (Store{DB: ApiTypes.ProjectDBHandle}).ResolveGrounding(c.Request().Context(), profileID, nodeID, in)
	if err != nil {
		status, code := http.StatusInternalServerError, "failed"
		switch {
		case errors.Is(err, ErrNodeNotFound):
			status, code = http.StatusNotFound, "missing"
		case errors.Is(err, errGroundingInput):
			status, code = http.StatusBadRequest, "invalid"
		case errors.Is(err, errGroundingConflict):
			status, code = http.StatusConflict, "stale"
		case errors.Is(err, errGroundingDuplicate):
			status, code = http.StatusConflict, "duplicate"
		case errors.Is(err, errGroundingObject):
			status, code = http.StatusConflict, "object"
		}
		rc.GetLogger().Error("product scope grounding failed", "profile_id", profileID, "node_id", nodeID, "error", err)
		return c.JSON(status, map[string]any{"status": false, "error_code": code})
	}
	rc.GetLogger().Info("product scope grounded", "profile_id", profileID, "node_id", nodeID, "object_id", objectID, "created", in.Create)
	return c.JSON(http.StatusOK, map[string]any{"status": true, "object_id": objectID})
}
