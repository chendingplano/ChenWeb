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
