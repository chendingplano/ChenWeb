package kbhandler

import (
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestListReferencesNullableTableRows(t *testing.T) {
	for _, test := range []struct {
		name  string
		value driver.Value
		want  string
	}{
		{"null", nil, ""},
		{"table", []byte(`[{"line":116,"rows":["r1"]}]`), `[{"line":116,"rows":["r1"]}]`},
		{"empty", []byte(`[]`), `[]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			old := ApiTypes.ProjectDBHandle
			ApiTypes.ProjectDBHandle = db
			defer func() { ApiTypes.ProjectDBHandle = old }()
			rows := sqlmock.NewRows([]string{"id", "ref_id", "section", "source_line_spans", "evidence", "cited_raw", "source_table_rows", "target_type", "target_subtype", "target_title", "target_version", "target_content", "normativity", "list_kind", "list_mismatch", "r1_state", "r2_state", "target_input_id", "target_input_title", "metric_state", "resource_title", "identity_key", "identifiers", "target_locations"}).
				AddRow(int64(1), "416_ref_1", "", `["116"]`, "Evidence", "ISO 9001", test.value, "standard", "international_standard", "", "", "general", "unlisted", "", "", "", "", int64(500), "", "", "", "", `[]`, `[{"source_line_spans":["210:211"],"source_table_rows":[{"line":212,"rows":["r1"]}]}]`)
			mock.ExpectQuery(`(?s)SELECT m.id, m.ref_id.*l.target_input_id = m.target_input_id.*l.status IN \('resolved', 'confirmed'\)`).WithArgs(int64(416)).WillReturnRows(rows)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/kb/references?input_record_id=416", nil), rec)
			if err = ListReferences(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var response listReferencesResponse
			if err = json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if !response.Status || len(response.Results) != 1 || string(response.Results[0].SourceTableRows) != test.want {
				t.Fatalf("unexpected response: %s", rec.Body.String())
			}
			var payload struct {
				Results []struct {
					TargetLocations []struct {
						SourceLineSpans []string        `json:"source_line_spans"`
						SourceTableRows json.RawMessage `json:"source_table_rows"`
					} `json:"target_locations"`
				} `json:"results"`
			}
			if err = json.Unmarshal(rec.Body.Bytes(), &payload); err != nil || len(payload.Results[0].TargetLocations) != 1 || payload.Results[0].TargetLocations[0].SourceLineSpans[0] != "210:211" {
				t.Fatalf("target PDF locations missing: %s", rec.Body.String())
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
