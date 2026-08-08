package docprocessing

import (
	"testing"
)

func TestParseDocProcessingPolicySeedConfig_TwoPoliciesAndOneBinding(t *testing.T) {
	data := []byte(`
[doc-processing-policy-no-entities-relations]
description = "Default policy"
is_default = true
processors = ["extract_metrics", "generate_topics"]

[doc-processing-policy-all]
description = "All processors"
is_default = false
processors = ["extract_metrics", "extract_entity", "extract_relation", "generate_topics"]

[doc-processing-policy-bindings]
Research = "doc-processing-policy-all"
`)
	cfg, err := ParseDocProcessingPolicySeedConfig(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.Policies) != 2 {
		t.Fatalf("expected 2 policies, got %d: %v", len(cfg.Policies), cfg.Policies)
	}
	noEnt, ok := cfg.Policies["no-entities-relations"]
	if !ok {
		t.Fatalf("expected policy %q, got keys %v", "no-entities-relations", cfg.Policies)
	}
	if !noEnt.IsDefault {
		t.Errorf("expected no-entities-relations.IsDefault = true")
	}
	if noEnt.Description != "Default policy" {
		t.Errorf("description = %q, want %q", noEnt.Description, "Default policy")
	}
	if len(noEnt.Processors) != 2 || noEnt.Processors[0] != "extract_metrics" || noEnt.Processors[1] != "generate_topics" {
		t.Errorf("processors = %v, want [extract_metrics generate_topics]", noEnt.Processors)
	}
	all, ok := cfg.Policies["all"]
	if !ok || all.IsDefault {
		t.Fatalf("expected policy %q with IsDefault = false, got %+v (ok=%v)", "all", all, ok)
	}
	if len(cfg.Bindings) != 1 || cfg.Bindings["Research"] != "all" {
		t.Errorf("bindings = %v, want map[Research:all]", cfg.Bindings)
	}
}

func TestParseDocProcessingPolicySeedConfig_RejectsNonStringDescription(t *testing.T) {
	data := []byte(`
[doc-processing-policy-x]
description = 123
is_default = true
processors = ["extract_metrics"]
`)
	if _, err := ParseDocProcessingPolicySeedConfig(data); err == nil {
		t.Fatal("expected an error for non-string description")
	}
}

func TestParseDocProcessingPolicySeedConfig_RejectsNonStringBindingValue(t *testing.T) {
	data := []byte(`
[doc-processing-policy-x]
description = "x"
is_default = true
processors = ["extract_metrics"]

[doc-processing-policy-bindings]
Research = 123
`)
	if _, err := ParseDocProcessingPolicySeedConfig(data); err == nil {
		t.Fatal("expected an error for non-string binding value")
	}
}

func TestParseDocProcessingPolicySeedConfig_IgnoresUnrelatedSections(t *testing.T) {
	data := []byte(`
[languages]
languages = ["en", "zh-cn"]
default = "zh-cn"

[doc-processing-policy-x]
description = "x"
is_default = true
processors = ["extract_metrics"]
`)
	cfg, err := ParseDocProcessingPolicySeedConfig(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.Policies) != 1 {
		t.Fatalf("expected 1 policy, got %d: %v", len(cfg.Policies), cfg.Policies)
	}
}

func TestLoadDocProcessingPolicySeedConfig_RealConfigFile(t *testing.T) {
	// ../../.. from server/api/doc-processing reaches the ChenWeb repo root,
	// where config.local.toml lives (Task 1 fixed its syntax and added the
	// bindings table this test relies on).
	cfg, err := LoadDocProcessingPolicySeedConfig("../../../config.local.toml")
	if err != nil {
		t.Fatalf("load real config.local.toml: %v", err)
	}
	if len(cfg.Policies) < 2 {
		t.Fatalf("expected at least 2 policies in config.local.toml, got %d: %v", len(cfg.Policies), cfg.Policies)
	}
	if _, ok := cfg.Policies["no-entities-relations"]; !ok {
		t.Errorf("expected policy %q in config.local.toml, got keys %v", "no-entities-relations", cfg.Policies)
	}
	if _, ok := cfg.Policies["all"]; !ok {
		t.Errorf("expected policy %q in config.local.toml, got keys %v", "all", cfg.Policies)
	}
	if cfg.Bindings["Research"] != "all" {
		t.Errorf("expected Research -> all binding, got %v", cfg.Bindings)
	}
}

func TestLoadDocProcessingPolicySeedConfig_MissingFile(t *testing.T) {
	if _, err := LoadDocProcessingPolicySeedConfig("does/not/exist.toml"); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestDocProcessingPolicySeedConfigValidate_Valid(t *testing.T) {
	cfg := DocProcessingPolicySeedConfig{
		Policies: map[string]DocProcessingPolicySeedPolicy{
			"no-entities-relations": {IsDefault: true, Processors: []string{"extract_metrics", "generate_topics"}},
			"all":                   {IsDefault: false, Processors: []string{"extract_metrics", "extract_entity"}},
		},
		Bindings: map[string]string{"Research": "all"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
	if got := cfg.DefaultPolicyName(); got != "no-entities-relations" {
		t.Errorf("DefaultPolicyName() = %q, want %q", got, "no-entities-relations")
	}
}

func TestDocProcessingPolicySeedConfigValidate_NoDefault(t *testing.T) {
	cfg := DocProcessingPolicySeedConfig{
		Policies: map[string]DocProcessingPolicySeedPolicy{
			"a": {IsDefault: false, Processors: []string{"extract_metrics"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error when no policy has is_default = true")
	}
}

func TestDocProcessingPolicySeedConfigValidate_TwoDefaults(t *testing.T) {
	cfg := DocProcessingPolicySeedConfig{
		Policies: map[string]DocProcessingPolicySeedPolicy{
			"a": {IsDefault: true, Processors: []string{"extract_metrics"}},
			"b": {IsDefault: true, Processors: []string{"extract_metrics"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error when two policies have is_default = true")
	}
}

func TestDocProcessingPolicySeedConfigValidate_EmptyProcessors(t *testing.T) {
	cfg := DocProcessingPolicySeedConfig{
		Policies: map[string]DocProcessingPolicySeedPolicy{
			"a": {IsDefault: true, Processors: nil},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error for an empty processors list")
	}
}

func TestDocProcessingPolicySeedConfigValidate_UnknownProcessor(t *testing.T) {
	cfg := DocProcessingPolicySeedConfig{
		Policies: map[string]DocProcessingPolicySeedPolicy{
			"a": {IsDefault: true, Processors: []string{"not_a_real_processor"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error for an unknown processor name")
	}
}

func TestDocProcessingPolicySeedConfigValidate_UnknownBindingPolicy(t *testing.T) {
	cfg := DocProcessingPolicySeedConfig{
		Policies: map[string]DocProcessingPolicySeedPolicy{
			"a": {IsDefault: true, Processors: []string{"extract_metrics"}},
		},
		Bindings: map[string]string{"Research": "does-not-exist"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error for a binding referencing an unknown policy")
	}
}

func TestDocProcessingPolicySeedConfigValidate_NoPolicies(t *testing.T) {
	cfg := DocProcessingPolicySeedConfig{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error for zero policies")
	}
}
