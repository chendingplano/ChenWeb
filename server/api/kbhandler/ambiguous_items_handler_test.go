package kbhandler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/attention"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/labstack/echo/v4"
)

func ambiguousItemContext(id string) (echo.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/kb/objects/ambiguous-items/"+id+"/artifact-object", nil), rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

func TestAmbiguousItemArtifactObjectReturnsNewestLiveRow(t *testing.T) {
	var gotOcc []docprocessing.TieOccurrence
	deps := ambiguousItemDeps{
		getItem: func(_ context.Context, id int64) (attention.Item, error) {
			return attention.Item{ID: id, ItemType: attention.TypeAmbiguousObjectName, Details: json.RawMessage(`{
				"occurrences": [
					{"input_record_id": 400, "artifact_type": "metric", "artifact_id": "400_mtc_1", "object_name": "管路", "seen_at": "2026-11-17T00:00:00Z"},
					{"input_record_id": 279, "artifact_type": "metric", "artifact_id": "279_mtc_1", "object_name": "管路", "seen_at": "2026-11-16T00:00:00Z"}
				]}`)}, nil
		},
		findRow: func(_ context.Context, occ []docprocessing.TieOccurrence) (int64, bool, error) {
			gotOcc = occ
			return 77, true, nil
		},
	}
	c, rec := ambiguousItemContext("5")
	if err := ambiguousItemArtifactObject(c, deps); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body)
	}
	var body struct {
		Found            bool  `json:"found"`
		ArtifactObjectID int64 `json:"artifact_object_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || !body.Found || body.ArtifactObjectID != 77 {
		t.Fatalf("body = %s", rec.Body)
	}
	if len(gotOcc) != 2 || gotOcc[0].InputRecordID != 400 || gotOcc[1].ArtifactID != "279_mtc_1" {
		t.Fatalf("occurrences passed = %+v, want newest first", gotOcc)
	}
}

func TestAmbiguousItemArtifactObjectErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   string
		item attention.Item
		err  error
		want int
	}{
		{"bad id", "x", attention.Item{}, nil, http.StatusBadRequest},
		{"not found", "5", attention.Item{}, attention.ErrNotFound, http.StatusNotFound},
		{"other type", "5", attention.Item{ItemType: "something-else", Details: json.RawMessage(`{}`)}, nil, http.StatusBadRequest},
	} {
		deps := ambiguousItemDeps{
			getItem: func(context.Context, int64) (attention.Item, error) { return tc.item, tc.err },
			findRow: func(context.Context, []docprocessing.TieOccurrence) (int64, bool, error) { return 0, false, nil },
		}
		c, rec := ambiguousItemContext(tc.id)
		if err := ambiguousItemArtifactObject(c, deps); err != nil {
			t.Fatal(err)
		}
		if rec.Code != tc.want {
			t.Fatalf("%s: code = %d", tc.name, rec.Code)
		}
	}
}
