package docprocessing

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPipelineRegistrySQLStoreListPipelinesReadsAuthoredSpecs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	query := regexp.QuoteMeta(`
SELECT name,
       COALESCE(display_name, ''),
       processors,
       legacy_equivalent
FROM kb.pipelines
ORDER BY id`)
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{
		"name", "display_name", "processors", "legacy_equivalent",
	}).AddRow(
		"legacy_default", "Legacy Default", `{}`, true,
	).AddRow(
		"narrative_default", "Narrative Default", `{extract_metrics,extract_provisions}`, false,
	))

	store := PipelineRegistrySQLStore{DB: db}
	got, err := store.ListPipelines(context.Background())
	if err != nil {
		t.Fatalf("ListPipelines: %v", err)
	}
	want := []ProductionPipelineSpec{
		{Name: "legacy_default", DisplayName: "Legacy Default", Processors: []string{}, LegacyEquivalent: true},
		{Name: "narrative_default", DisplayName: "Narrative Default", Processors: []string{"extract_metrics", "extract_provisions"}, LegacyEquivalent: false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d specs, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Name != want[i].Name || got[i].DisplayName != want[i].DisplayName || got[i].LegacyEquivalent != want[i].LegacyEquivalent {
			t.Fatalf("spec[%d]=%#v want=%#v", i, got[i], want[i])
		}
		if len(got[i].Processors) != len(want[i].Processors) {
			t.Fatalf("spec[%d].Processors=%#v want=%#v", i, got[i].Processors, want[i].Processors)
		}
		for j := range want[i].Processors {
			if got[i].Processors[j] != want[i].Processors[j] {
				t.Fatalf("spec[%d].Processors=%#v want=%#v", i, got[i].Processors, want[i].Processors)
			}
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestPipelineRegistrySQLStoreListPipelinesRejectsNilDB(t *testing.T) {
	store := PipelineRegistrySQLStore{}
	if _, err := store.ListPipelines(context.Background()); err == nil {
		t.Fatal("expected error for nil db")
	}
}

func TestLoadProductionPipelineRegistryInstallsAuthoredSpecs(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRegistry(nil) })

	if err := LoadProductionPipelineRegistry(context.Background(), fakePipelineRegistryStore{
		specs: []ProductionPipelineSpec{{Name: "authored_default", LegacyEquivalent: true}},
	}); err != nil {
		t.Fatalf("LoadProductionPipelineRegistry: %v", err)
	}

	if _, ok := LookupProductionPipeline("authored_default"); !ok {
		t.Fatal("expected authored_default to be installed")
	}
	if _, ok := LookupProductionPipeline("legacy_default"); ok {
		t.Fatal("legacy_default should not resolve once the authored registry replaced the fallback")
	}
}

func TestLoadProductionPipelineRegistryLeavesRegistryUnchangedOnError(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRegistry(nil) })

	SetProductionPipelineRegistry([]ProductionPipelineSpec{{Name: "already_installed", LegacyEquivalent: true}})

	if err := LoadProductionPipelineRegistry(context.Background(), fakePipelineRegistryStore{err: errFakePipelineStore}); err == nil {
		t.Fatal("expected error from failing store")
	}
	if _, ok := LookupProductionPipeline("already_installed"); !ok {
		t.Fatal("expected previously installed registry to remain in place after a failed load")
	}

	if err := LoadProductionPipelineRegistry(context.Background(), fakePipelineRegistryStore{}); err == nil {
		t.Fatal("expected error for empty authored registry")
	}
	if _, ok := LookupProductionPipeline("already_installed"); !ok {
		t.Fatal("expected previously installed registry to remain in place after an empty load")
	}
}

func TestLoadProductionPipelineRegistryRejectsNilStore(t *testing.T) {
	if err := LoadProductionPipelineRegistry(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil store")
	}
}

var errFakePipelineStore = errors.New("pipeline store failure")

type fakePipelineRegistryStore struct {
	specs []ProductionPipelineSpec
	err   error
}

func (f fakePipelineRegistryStore) ListPipelines(context.Context) ([]ProductionPipelineSpec, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.specs, nil
}
