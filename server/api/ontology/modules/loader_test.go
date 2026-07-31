package modules

import "testing"

func TestActiveModuleRegistryRoundTrip(t *testing.T) {
	SetActiveModules(map[string]ActiveModule{
		"core":     {ModuleID: "core", ReleaseID: 1, Version: "1.0.0", Checksum: "c1"},
		"quantity": {ModuleID: "quantity", ReleaseID: 2, Version: "0.1.0", Checksum: "c2"},
	})

	if m, ok := GetActiveModule("core"); !ok || m.Version != "1.0.0" {
		t.Fatalf("unexpected core active module: %#v ok=%v", m, ok)
	}
	if _, ok := GetActiveModule("ghost"); ok {
		t.Fatal("expected no active module for ghost")
	}
	ids := ActiveModuleIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 active module ids, got %v", ids)
	}
}
