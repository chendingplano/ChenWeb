package docprocessing

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestProcessorRegistrySQLStoreListProcessorRegistryReadsRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	query := regexp.QuoteMeta(`
SELECT name, phase, class, cost, on_undetermined, idempotent, requires, produces
FROM kb.processor_registry
ORDER BY name`)
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{
		"name", "phase", "class", "cost", "on_undetermined", "idempotent", "requires", "produces",
	}).AddRow(
		"extract_widgets", "B", "routed", "cheap_llm", "skip", true, `{chunks}`, `{widgets}`,
	))

	store := ProcessorRegistrySQLStore{DB: db}
	got, err := store.ListProcessorRegistry(context.Background())
	if err != nil {
		t.Fatalf("ListProcessorRegistry: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d specs, want 1: %#v", len(got), got)
	}
	spec := got[0]
	if spec.Name != "extract_widgets" || spec.Phase != "B" || spec.Class != "routed" || spec.Cost != "cheap_llm" || spec.OnUndetermined != "skip" || !spec.Idempotent {
		t.Fatalf("unexpected spec: %#v", spec)
	}
	if len(spec.Requires) != 1 || spec.Requires[0] != "chunks" || len(spec.Produces) != 1 || spec.Produces[0] != "widgets" {
		t.Fatalf("unexpected requires/produces: %#v", spec)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestProcessorRegistrySQLStoreListProcessorRegistryRejectsNilDB(t *testing.T) {
	store := ProcessorRegistrySQLStore{}
	if _, err := store.ListProcessorRegistry(context.Background()); err == nil {
		t.Fatal("expected error for nil db")
	}
}

func TestLoadProcessorRegistryRegistersUnknownProcessor(t *testing.T) {
	t.Cleanup(func() { RegisterProcessor(ProcessorSpec{Name: "extract_metrics", Phase: "B", DependsOn: []string{"chunking"}, Requires: []string{"chunks"}, Produces: []string{"metrics"}}) })

	if err := LoadProcessorRegistry(context.Background(), fakeProcessorRegistryStore{
		specs: []ProcessorSpec{{Name: "extract_widgets", Phase: "B", Requires: []string{"chunks"}, Produces: []string{"widgets"}}},
	}); err != nil {
		t.Fatalf("LoadProcessorRegistry: %v", err)
	}

	spec, ok := LookupProcessor("extract_widgets")
	if !ok {
		t.Fatal("expected extract_widgets to be registered from kb.processor_registry")
	}
	if len(spec.Produces) != 1 || spec.Produces[0] != "widgets" {
		t.Fatalf("unexpected registered spec: %#v", spec)
	}
}

func TestLoadProcessorRegistryHardcodedSpecWins(t *testing.T) {
	// A registry row for a name already in productionProcessorSpecs must
	// never override the hardcoded spec (DR7's union, hardcoded side wins).
	original, ok := LookupProcessor("extract_metrics")
	if !ok {
		t.Fatal("expected extract_metrics to already be seeded from productionProcessorSpecs")
	}

	if err := LoadProcessorRegistry(context.Background(), fakeProcessorRegistryStore{
		specs: []ProcessorSpec{{Name: "extract_metrics", Phase: "Z", Cost: "expensive_llm"}},
	}); err != nil {
		t.Fatalf("LoadProcessorRegistry: %v", err)
	}

	got, _ := LookupProcessor("extract_metrics")
	if got.Phase != original.Phase || got.Cost != original.Cost {
		t.Fatalf("hardcoded spec was overwritten by registry row: got %#v, want %#v", got, original)
	}
}

func TestLoadProcessorRegistryToleratesEmptyTable(t *testing.T) {
	if err := LoadProcessorRegistry(context.Background(), fakeProcessorRegistryStore{}); err != nil {
		t.Fatalf("LoadProcessorRegistry with empty registry table should not error: %v", err)
	}
}

func TestLoadProcessorRegistryRejectsNilStore(t *testing.T) {
	if err := LoadProcessorRegistry(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil store")
	}
}

func TestLoadProcessorRegistryPropagatesStoreError(t *testing.T) {
	if err := LoadProcessorRegistry(context.Background(), fakeProcessorRegistryStore{err: errFakeProcessorRegistryStore}); err == nil {
		t.Fatal("expected error from failing store")
	}
}

var errFakeProcessorRegistryStore = errors.New("processor registry store failure")

type fakeProcessorRegistryStore struct {
	specs []ProcessorSpec
	err   error
}

func (f fakeProcessorRegistryStore) ListProcessorRegistry(context.Context) ([]ProcessorSpec, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.specs, nil
}
