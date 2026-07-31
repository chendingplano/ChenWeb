package modules

import "testing"

func TestValidateDepsRejectsCycles(t *testing.T) {
	mods := []Module{
		{ModuleID: "a", DependsOn: []string{"b"}},
		{ModuleID: "b", DependsOn: []string{"c"}},
		{ModuleID: "c", DependsOn: []string{"a"}},
	}
	if err := validateDeps(mods); err == nil {
		t.Fatal("expected cycle detection")
	}
}

func TestValidateDepsRejectsUnknownDependency(t *testing.T) {
	mods := []Module{
		{ModuleID: "a", DependsOn: []string{"ghost"}},
	}
	if err := validateDeps(mods); err == nil {
		t.Fatal("expected unknown dependency error")
	}
}

func TestValidateDepsAcceptsDAG(t *testing.T) {
	mods := []Module{
		{ModuleID: "core", DependsOn: []string{}},
		{ModuleID: "quantity", DependsOn: []string{"core"}},
		{ModuleID: "measurement", DependsOn: []string{"core", "quantity"}},
	}
	if err := validateDeps(mods); err != nil {
		t.Fatalf("expected valid DAG, got %v", err)
	}
}

func TestChecksumDeterministicAcrossKeyOrder(t *testing.T) {
	a := []byte(`{"module_id":"core","terms":[{"term_id":"core:assertion","definition":"x"}]}`)
	b := []byte(`{"terms":[{"definition":"x","term_id":"core:assertion"}],"module_id":"core"}`)
	ca, err := Checksum(a)
	if err != nil {
		t.Fatalf("Checksum: %v", err)
	}
	cb, err := Checksum(b)
	if err != nil {
		t.Fatalf("Checksum: %v", err)
	}
	if ca != cb {
		t.Fatalf("checksum depends on key order: %s != %s", ca, cb)
	}
}

func TestChecksumDiffersOnContentChange(t *testing.T) {
	a, _ := Checksum([]byte(`{"terms":[{"term_id":"core:assertion"}]}`))
	b, _ := Checksum([]byte(`{"terms":[{"term_id":"core:evidence"}]}`))
	if a == b {
		t.Fatal("expected different checksums for different content")
	}
}
