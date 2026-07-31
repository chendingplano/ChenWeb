package docprocessing

import (
	"reflect"
	"testing"
)

func TestOptionalProductionProcessorNames(t *testing.T) {
	want := []string{
		"generate_summaries", "generate_topics", "extract_doc_metadata",
		"extract_semantic_projections", "extract_structured_knowledge",
		"extract_entity", "extract_relation", "extract_inventory_items",
		"extract_metrics", "extract_provisions", "generate_scene_blocks",
	}
	got := OptionalProductionProcessorNames()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	got[0] = "mutated"
	if OptionalProductionProcessorNames()[0] != "generate_summaries" {
		t.Fatal("registry returned shared mutable storage")
	}
}

func TestCanonicalOptionalProductionProcessor(t *testing.T) {
	got, ok := CanonicalOptionalProductionProcessor(" extract-metadata ")
	if !ok || got != "extract_doc_metadata" {
		t.Fatalf("got %q, %v", got, ok)
	}
	if _, ok := CanonicalOptionalProductionProcessor("static_analyzer"); ok {
		t.Fatal("mandatory processor must not be selectable")
	}
	if _, ok := CanonicalOptionalProductionProcessor("not_a_processor"); ok {
		t.Fatal("unknown processor accepted")
	}
}
