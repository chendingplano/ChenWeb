package assertions

import (
	"context"
	"database/sql"
	"testing"
)

func TestProjectionBuilderRegistryRejectsDuplicateKind(t *testing.T) {
	r := &ProjectionBuilderRegistry{builders: map[string]projectionBuilderEntry{}}
	build := func(ctx context.Context, db *sql.DB, targetID string) error { return nil }
	if err := r.Register("test_kind", build, nil); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register("test_kind", build, nil); err == nil {
		t.Fatal("expected error registering a duplicate kind")
	}
}

func TestProjectionBuilderRegistryRequiresBuildFunc(t *testing.T) {
	r := &ProjectionBuilderRegistry{builders: map[string]projectionBuilderEntry{}}
	if err := r.Register("test_kind", nil, nil); err == nil {
		t.Fatal("expected error when build func is nil")
	}
}

func TestProjectionBuilderRegistryDefaultsRepairToBuild(t *testing.T) {
	r := &ProjectionBuilderRegistry{builders: map[string]projectionBuilderEntry{}}
	called := false
	build := func(ctx context.Context, db *sql.DB, targetID string) error {
		called = true
		return nil
	}
	if err := r.Register("test_kind", build, nil); err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, repair, ok := r.Lookup("test_kind")
	if !ok {
		t.Fatal("expected the kind to be registered")
	}
	fixed, err := repair(context.Background(), nil, "target-1")
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if !called {
		t.Fatal("expected the default repair to delegate to build")
	}
	if !fixed {
		t.Fatal("expected the default repair to report a fix occurred")
	}
}

func TestProjectionBuilderRegistryLookupMissingKind(t *testing.T) {
	r := &ProjectionBuilderRegistry{builders: map[string]projectionBuilderEntry{}}
	if _, _, ok := r.Lookup("nonexistent"); ok {
		t.Fatal("expected lookup of an unregistered kind to fail")
	}
}

func TestClassificationProjectionSelfRegistered(t *testing.T) {
	build, repair, ok := LookupProjectionBuilder(ProjectionKindObjectPrimaryClass)
	if !ok || build == nil || repair == nil {
		t.Fatal("expected the classification projection to self-register via init()")
	}
}

// TestClassificationProjectionRecordScopeSelfRegistered locks in the seam-7
// fix: ProjectSemantics.Run must be able to discover this kind's per-record
// target resolver generically, via RegisterProjectionRecordScope, not via
// project_semantics.go hardcoding ProjectionKindObjectPrimaryClass.
func TestClassificationProjectionRecordScopeSelfRegistered(t *testing.T) {
	targetTable, targetsForRecord, ok := LookupProjectionRecordScope(ProjectionKindObjectPrimaryClass)
	if !ok || targetsForRecord == nil {
		t.Fatal("expected the classification projection's record scope to self-register via init()")
	}
	if targetTable != "kb.object_nodes" {
		t.Fatalf("expected target table kb.object_nodes, got %q", targetTable)
	}
}

func TestRegisterProjectionRecordScopeRejectsDuplicateKind(t *testing.T) {
	fn := func(ctx context.Context, db *sql.DB, inputRecordID int64) ([]string, error) { return nil, nil }
	if err := RegisterProjectionRecordScope("test_record_scope_kind", "kb.test_targets", fn); err != nil {
		t.Fatalf("first RegisterProjectionRecordScope: %v", err)
	}
	if err := RegisterProjectionRecordScope("test_record_scope_kind", "kb.test_targets", fn); err == nil {
		t.Fatal("expected error registering a duplicate kind")
	}
}

func TestRegisterProjectionRecordScopeRequiresFields(t *testing.T) {
	fn := func(ctx context.Context, db *sql.DB, inputRecordID int64) ([]string, error) { return nil, nil }
	if err := RegisterProjectionRecordScope("", "kb.test_targets", fn); err == nil {
		t.Fatal("expected error when kind is empty")
	}
	if err := RegisterProjectionRecordScope("test_record_scope_kind_2", "", fn); err == nil {
		t.Fatal("expected error when target table is empty")
	}
	if err := RegisterProjectionRecordScope("test_record_scope_kind_3", "kb.test_targets", nil); err == nil {
		t.Fatal("expected error when targetsForRecord is nil")
	}
}

func TestLookupProjectionRecordScopeMissingKind(t *testing.T) {
	if _, _, ok := LookupProjectionRecordScope("nonexistent_record_scope_kind"); ok {
		t.Fatal("expected lookup of an unregistered kind to fail")
	}
}
