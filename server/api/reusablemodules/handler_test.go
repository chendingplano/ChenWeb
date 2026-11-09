package reusablemodules

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
)

func TestList(t *testing.T) {
	for _, scenario := range []string{"records", "empty", "database error", "scan error", "rows error"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			rows := sqlmock.NewRows([]string{"id", "module_id", "module_name", "module_type", "prog_language", "description", "files", "version", "config_json", "status", "doc_path", "capability", "capability_label", "use_when", "devdoc_paths", "created_by", "updated_by", "created_at", "updated_at"})
			now := time.Now().UTC()
			if scenario != "empty" {
				rows.AddRow(1, "20261025-01", "Metric Visualizer", "frontend", "svelte", "Shows metrics", `["ChenWeb/web/metric.svelte"]`, "1.0.0", `{"compact":true}`, "active", "KnowledgeStore/doc-repo/modules/module-metric.md", "record-viewers", "Record viewers", "show one metric", `["KnowledgeStore/doc-repo/devdocs/202610/2026102501-devdoc-x.md"]`, "cding", "cding", now, now)
			}
			expectation := mock.ExpectQuery(`SELECT .+ FROM kb\.reusable_modules r LEFT JOIN kb\.reusable_module_capabilities c .+ ORDER BY r\.module_name, r\.module_id`)
			if scenario == "database error" {
				expectation.WillReturnError(errors.New("private database details"))
			} else {
				if scenario == "scan error" {
					rows = sqlmock.NewRows([]string{"id"}).AddRow("invalid")
				}
				if scenario == "rows error" {
					rows.RowError(0, errors.New("private row error"))
				}
				expectation.WillReturnRows(rows)
			}
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest("GET", "/api/v1/reusable-modules", nil), rec)
			if err := Handler(db)(c); err != nil {
				t.Fatal(err)
			}
			if scenario == "database error" || scenario == "scan error" || scenario == "rows error" {
				if rec.Code != 500 {
					t.Fatalf("status = %d", rec.Code)
				}
				if rec.Body.String() != "{\"error\":\"Unable to load modules\"}\n" {
					t.Fatal(rec.Body.String())
				}
			} else {
				if rec.Code != 200 {
					t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
				}
				var response struct {
					Modules []Module `json:"modules"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.Modules == nil {
					t.Fatal("modules must be an array")
				}
				if scenario == "records" {
					if len(response.Modules) != 1 || response.Modules[0].ModuleID != "20261025-01" {
						t.Fatalf("unexpected modules: %+v", response.Modules)
					}
					if string(response.Modules[0].Files) != `["ChenWeb/web/metric.svelte"]` {
						t.Fatal("files must remain structured JSON")
					}
					if string(response.Modules[0].DevdocPaths) != `["KnowledgeStore/doc-repo/devdocs/202610/2026102501-devdoc-x.md"]` {
						t.Fatal("devdoc paths must remain structured JSON")
					}
					if response.Modules[0].Capability != "record-viewers" || response.Modules[0].CapabilityLabel != "Record viewers" || response.Modules[0].UseWhen != "show one metric" {
						t.Fatalf("index fields not returned: %+v", response.Modules[0])
					}
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
