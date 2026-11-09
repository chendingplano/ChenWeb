package docprocessing

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestProvisionV2PreservesNewObjectType(t *testing.T) {
	payload := extractProvisionTestPayload(t, strings.Replace(provisionV2Example, `"t":"mat"`, `"t":"Waste Stream"`, 1))
	object := payload["provisions"].([]any)[0].(map[string]any)["objects"].([]any)[0].(map[string]any)
	if object["object_type"] != "waste_stream" || object["object_type_raw"] != "Waste Stream" {
		t.Fatalf("new type was lost: %#v", object)
	}
}

func TestObjectTypeResolveRegistersProposalWithoutDowngrading(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	columns := []string{"id", "name", "status", "raw_value", "source_record_id", "source_run_id", "source_processor", "source_model", "source_prompt", "source_spans", "created_at", "approved_at", "approved_by"}
	mock.ExpectQuery("INSERT INTO kb.object_types").WithArgs("waste_stream", "Waste Stream", int64(224), int64(172), "extract_provisions", "model", "prompt", `["42"]`).WillReturnRows(sqlmock.NewRows(columns).AddRow(11, "waste_stream", "proposed", "Waste Stream", 224, 172, "extract_provisions", "model", "prompt", `["42"]`, now, nil, ""))
	got, err := (ObjectTypeSQLStore{DB: db}).Resolve(context.Background(), "Waste Stream", ObjectTypeSource{RecordID: 224, RunID: 172, Processor: "extract_provisions", Model: "model", Prompt: "prompt", Spans: []string{"42"}})
	if err != nil || got.Name != "waste_stream" || got.Status != "proposed" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestObjectTypeBlankCannotCreateProposal(t *testing.T) {
	_, err := (ObjectTypeSQLStore{}).Resolve(context.Background(), " \t ", ObjectTypeSource{})
	if err == nil {
		t.Fatal("blank type accepted")
	}
}

func TestTypeConfidenceRequiresApproval(t *testing.T) {
	if approvedObjectTypesCompatible("waste_stream", "waste_stream", map[string]bool{}) {
		t.Fatal("proposal boosted matching")
	}
	if !approvedObjectTypesCompatible("waste_stream", "waste_stream", map[string]bool{"waste_stream": true}) {
		t.Fatal("approval did not enable type compatibility")
	}
	if approvedObjectTypesCompatible("waste_stream", "other", map[string]bool{"other": true}) {
		t.Fatal("proposal treated as wildcard")
	}
}

func TestSQLCandidateConfidenceChangesAfterTypeApproval(t *testing.T) {
	for _, approved := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		columns := []string{"id", "object_id", "canonical_object_id", "canonical_name", "canonical_name_en", "canonical_name_zh", "primary_language", "object_type", "aliases", "acronyms", "normalized_names", "description", "search_document", "reconcile_status", "ext_info"}
		mock.ExpectQuery("FROM kb.object_nodes").WillReturnRows(sqlmock.NewRows(columns).AddRow(1, "node_1", "", "runoff", "", "", "", "waste_stream", `[]`, `[]`, `["runoff"]`, "", "", "active", `{}`))
		registryRows := sqlmock.NewRows([]string{"name"})
		if approved {
			registryRows.AddRow("waste_stream")
		}
		mock.ExpectQuery("SELECT name FROM kb.object_types").WillReturnRows(registryRows)
		candidates, err := (ObjectNodeSQLStore{DB: db}).FindCandidates(context.Background(), ArtifactObject{ObjectName: "runoff", ObjectType: "waste_stream", NormalizedNames: []string{"runoff"}}, ObjectReconcileOptions{})
		if err != nil {
			t.Fatal(err)
		}
		want := 0.85
		if approved {
			want = 1
		}
		if len(candidates) != 1 || candidates[0].Score != want {
			t.Fatalf("approved=%v candidates=%+v", approved, candidates)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}
