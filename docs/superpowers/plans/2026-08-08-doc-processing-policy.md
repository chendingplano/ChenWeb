# Knowledge-Store-Scoped Doc Processing Policies Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Author real "Doc Processing Policy" content — via a new `config.local.toml`-driven seed tool — into the already-built `kb.pipelines`/`kb.pipeline_bindings`/`kb.pipeline_policies` mechanism, so each knowledge store can run a different processor list with one configured default.

**Architecture:** One new package-internal config loader/validator in `server/api/doc-processing` parses `[doc-processing-policy-*]` TOML sections into a typed config and validates it (exactly one default, every processor name real, every binding's target policy exists). One new transactional seed function in the same package upserts `kb.pipelines` rows, authors a draft `kb.pipeline_policies` version with `store_default`-kind `kb.pipeline_bindings` rows (one system-wide default + one per bound store), compiles it via the existing `PolicyCompilerSQLStore`, and activates it — archiving whatever was active before. A new thin CLI, `server/cmd/doc-processing-policy-seed`, wires config file → DB, modeled directly on `server/cmd/ontology-seed`.

**Tech Stack:** Go 1.25, `github.com/pelletier/go-toml/v2` (already a dependency) for TOML parsing, `database/sql` + `github.com/lib/pq` for Postgres, `github.com/DATA-DOG/go-sqlmock` for store tests (matching this package's existing test convention, e.g. `policy_promotion_test.go`).

## Global Constraints

- Governing spec: `ChenWeb/docs/superpowers/specs/2026-08-08-doc-processing-policy-design.md`. Every task below implements one of its sections; do not add behavior the spec doesn't call for (no per-processor gates, no frontend, no hot-reload, no binding scopes beyond system/knowledge-store — spec §7).
- Follow `ChenWeb/CLAUDE.md`: minimum code that solves the problem, no speculative abstractions, surgical changes, match existing style exactly (this plan's code blocks already do — copy them as given).
- Commits go through `jj commit` (never `git commit` directly), one commit per task step as shown.
- No prompts are needed for this feature (no LLM calls) — the `prompts/` naming convention doesn't apply here.
- The package under modification is `docprocessing` at `server/api/doc-processing` (import path `github.com/chendingplano/deepdoc/server/api/doc-processing`).

---

### Task 1: Fix the config.local.toml policy sections

**Files:**
- Modify: `config.local.toml:132-156`

**Interfaces:**
- Produces: two syntactically valid `[doc-processing-policy-*]` TOML sections and one `[doc-processing-policy-bindings]` section for Task 2's parser to consume.

- [ ] **Step 1: Fix the stray `]` and add the bindings table**

Replace lines 132-156 (the two policy sections, which currently have an extra `]` after each `processors` array already closes, and are missing the bindings table the design calls for) with:

```toml
[doc-processing-policy-no-entities-relations]
description = "This is the system default policy. It contains all but entities and relations processors"
is_default = true
processors = [
	"extract_metrics",
	"extract_provisions",
	"extract_semantic_projections",
	"generate_topics",
	"generate_scene_blocks",
	"extract_inventory_items"]

[doc-processing-policy-all]
description = "This policy includes all the doc processors"
is_default = false
processors = [
	"extract_metrics",
	"extract_provisions",
	"extract_semantic_projections",
	"extract_entity",
	"extract_relation",
	"generate_topics",
	"generate_scene_blocks",
	"extract_inventory_items"]

[doc-processing-policy-bindings]
Research = "doc-processing-policy-all"
```

- [ ] **Step 2: Verify the file is valid TOML**

This is confirmed by Task 2's `TestLoadDocProcessingPolicySeedConfig_RealConfigFile`, which parses this exact file. No separate check needed now — the plan's next task exercises it directly.

- [ ] **Step 3: Commit**

```bash
jj commit -m "config: fix doc-processing-policy TOML syntax and add store bindings"
```

---

### Task 2: Config parsing (`DocProcessingPolicySeedConfig`)

**Files:**
- Create: `server/api/doc-processing/policy_seed_config.go`
- Test: `server/api/doc-processing/policy_seed_config_test.go`

**Interfaces:**
- Produces:
  - `type DocProcessingPolicySeedPolicy struct { Description string; IsDefault bool; Processors []string }`
  - `type DocProcessingPolicySeedConfig struct { Policies map[string]DocProcessingPolicySeedPolicy; Bindings map[string]string }` (`Bindings` maps knowledge-store name → policy name)
  - `func ParseDocProcessingPolicySeedConfig(data []byte) (DocProcessingPolicySeedConfig, error)`
  - `func LoadDocProcessingPolicySeedConfig(path string) (DocProcessingPolicySeedConfig, error)`
- Consumes: nothing from other tasks.

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server/api/doc-processing && go test ./... -run TestParseDocProcessingPolicySeedConfig -v`
Expected: FAIL with `undefined: ParseDocProcessingPolicySeedConfig` (and similarly for `LoadDocProcessingPolicySeedConfig`).

- [ ] **Step 3: Write the implementation**

```go
// policy_seed_config.go
package docprocessing

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// DocProcessingPolicySeedPolicy is one [doc-processing-policy-<name>]
// section: a named, human-described processor allow-list, sourced from
// config.local.toml (see docs/superpowers/specs/2026-08-08-doc-processing-policy-design.md).
type DocProcessingPolicySeedPolicy struct {
	Description string
	IsDefault   bool
	Processors  []string
}

// DocProcessingPolicySeedConfig is every [doc-processing-policy-*] section
// in a config file, keyed by the section-name suffix after
// "doc-processing-policy-" (e.g. "no-entities-relations", "all"), plus the
// [doc-processing-policy-bindings] knowledge-store-name -> policy-name map.
type DocProcessingPolicySeedConfig struct {
	Policies map[string]DocProcessingPolicySeedPolicy
	Bindings map[string]string
}

const (
	docProcessingPolicySeedSectionPrefix = "doc-processing-policy-"
	docProcessingPolicySeedBindingsKey   = docProcessingPolicySeedSectionPrefix + "bindings"
)

// LoadDocProcessingPolicySeedConfig reads and parses path.
func LoadDocProcessingPolicySeedConfig(path string) (DocProcessingPolicySeedConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DocProcessingPolicySeedConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseDocProcessingPolicySeedConfig(data)
}

// ParseDocProcessingPolicySeedConfig parses raw TOML bytes. Sections not
// prefixed with "doc-processing-policy-" are ignored, so the same file can
// hold unrelated config (as config.local.toml does).
func ParseDocProcessingPolicySeedConfig(data []byte) (DocProcessingPolicySeedConfig, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return DocProcessingPolicySeedConfig{}, fmt.Errorf("parse toml: %w", err)
	}
	cfg := DocProcessingPolicySeedConfig{
		Policies: map[string]DocProcessingPolicySeedPolicy{},
		Bindings: map[string]string{},
	}
	for key, value := range raw {
		if !strings.HasPrefix(key, docProcessingPolicySeedSectionPrefix) {
			continue
		}
		section, ok := value.(map[string]any)
		if !ok {
			return DocProcessingPolicySeedConfig{}, fmt.Errorf("%s: expected a table", key)
		}
		if key == docProcessingPolicySeedBindingsKey {
			for store, policyVal := range section {
				policyName, ok := policyVal.(string)
				if !ok {
					return DocProcessingPolicySeedConfig{}, fmt.Errorf("%s.%s: expected a string policy name", key, store)
				}
				cfg.Bindings[store] = strings.TrimPrefix(policyName, docProcessingPolicySeedSectionPrefix)
			}
			continue
		}
		name := strings.TrimPrefix(key, docProcessingPolicySeedSectionPrefix)
		policy, err := decodeDocProcessingPolicySeedPolicy(section)
		if err != nil {
			return DocProcessingPolicySeedConfig{}, fmt.Errorf("%s: %w", key, err)
		}
		cfg.Policies[name] = policy
	}
	return cfg, nil
}

func decodeDocProcessingPolicySeedPolicy(section map[string]any) (DocProcessingPolicySeedPolicy, error) {
	var policy DocProcessingPolicySeedPolicy
	if v, ok := section["description"]; ok {
		s, ok := v.(string)
		if !ok {
			return DocProcessingPolicySeedPolicy{}, fmt.Errorf("description must be a string")
		}
		policy.Description = s
	}
	if v, ok := section["is_default"]; ok {
		b, ok := v.(bool)
		if !ok {
			return DocProcessingPolicySeedPolicy{}, fmt.Errorf("is_default must be a boolean")
		}
		policy.IsDefault = b
	}
	if v, ok := section["processors"]; ok {
		list, ok := v.([]any)
		if !ok {
			return DocProcessingPolicySeedPolicy{}, fmt.Errorf("processors must be an array")
		}
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return DocProcessingPolicySeedPolicy{}, fmt.Errorf("processors entries must be strings")
			}
			policy.Processors = append(policy.Processors, s)
		}
	}
	return policy, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server/api/doc-processing && go test ./... -run TestParseDocProcessingPolicySeedConfig -v && go test ./... -run TestLoadDocProcessingPolicySeedConfig -v`
Expected: PASS (all 6 tests).

- [ ] **Step 5: Commit**

```bash
jj commit -m "doc-processing: add doc-processing-policy TOML config parser"
```

---

### Task 3: Config validation

**Files:**
- Modify: `server/api/doc-processing/policy_seed_config.go`
- Test: `server/api/doc-processing/policy_seed_config_test.go`

**Interfaces:**
- Consumes: `DocProcessingPolicySeedConfig`, `DocProcessingPolicySeedPolicy` (Task 2); package-private `validateRequiredProcessors([]string) error` (existing, `runtime.go:367`).
- Produces:
  - `func (c DocProcessingPolicySeedConfig) Validate() error`
  - `func (c DocProcessingPolicySeedConfig) DefaultPolicyName() string` (returns `""` if `Validate` would fail — callers must call `Validate` first)

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server/api/doc-processing && go test ./... -run TestDocProcessingPolicySeedConfigValidate -v`
Expected: FAIL with `cfg.Validate undefined` / `cfg.DefaultPolicyName undefined`.

- [ ] **Step 3: Write the implementation**

Append to `policy_seed_config.go`:

```go
// Validate enforces the doc-processing-policy-seed rules: at least one
// policy, exactly one is_default = true, every processor name resolves
// against the real production processor registry, and every binding names
// a policy that exists in this same config.
func (c DocProcessingPolicySeedConfig) Validate() error {
	if len(c.Policies) == 0 {
		return fmt.Errorf("no [doc-processing-policy-*] sections found")
	}
	var defaultName string
	for name, policy := range c.Policies {
		if len(policy.Processors) == 0 {
			return fmt.Errorf("policy %q: processors must not be empty", name)
		}
		if err := validateRequiredProcessors(policy.Processors); err != nil {
			return fmt.Errorf("policy %q: %w", name, err)
		}
		if policy.IsDefault {
			if defaultName != "" {
				return fmt.Errorf("multiple default policies: %q and %q", defaultName, name)
			}
			defaultName = name
		}
	}
	if defaultName == "" {
		return fmt.Errorf("no policy has is_default = true")
	}
	for store, policyName := range c.Bindings {
		if _, ok := c.Policies[policyName]; !ok {
			return fmt.Errorf("binding %q: unknown policy %q", store, policyName)
		}
	}
	return nil
}

// DefaultPolicyName returns the name of the policy with IsDefault = true.
// Callers must call Validate first; DefaultPolicyName returns "" if no
// policy is marked default (Validate would have already rejected that).
func (c DocProcessingPolicySeedConfig) DefaultPolicyName() string {
	for name, policy := range c.Policies {
		if policy.IsDefault {
			return name
		}
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server/api/doc-processing && go test ./... -run TestDocProcessingPolicySeedConfigValidate -v`
Expected: PASS (all 7 tests).

- [ ] **Step 5: Commit**

```bash
jj commit -m "doc-processing: validate doc-processing-policy config"
```

---

### Task 4: Seed transaction (`SeedDocProcessingPolicies`)

**Files:**
- Create: `server/api/doc-processing/policy_seed.go`
- Test: `server/api/doc-processing/policy_seed_test.go`

**Interfaces:**
- Consumes: `DocProcessingPolicySeedConfig`/`DocProcessingPolicySeedPolicy`/`Validate`/`DefaultPolicyName` (Tasks 2-3); existing `PolicyCompilerSQLStore{DB policyCompileQuerier}.CompilePolicy(ctx context.Context, policyID int64) (CompiledPolicy, error)` (`policy_compile.go:217`), where `CompiledPolicy.Checksum string`.
- Produces:
  - `type DocProcessingPolicySeedResult struct { PipelinesCreated, PipelinesUpdated []string; PolicyID int64; PolicyVersion int; BindingsWritten int }`
  - `func SeedDocProcessingPolicies(ctx context.Context, db *sql.DB, cfg DocProcessingPolicySeedConfig) (DocProcessingPolicySeedResult, error)`

- [ ] **Step 1: Write the failing tests**

```go
// policy_seed_test.go
package docprocessing

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func validSeedConfig() DocProcessingPolicySeedConfig {
	return DocProcessingPolicySeedConfig{
		Policies: map[string]DocProcessingPolicySeedPolicy{
			"no-entities-relations": {Description: "Default", IsDefault: true, Processors: []string{"extract_metrics", "generate_topics"}},
			"all":                   {Description: "All", IsDefault: false, Processors: []string{"extract_metrics", "extract_entity"}},
		},
		Bindings: map[string]string{"Research": "all"},
	}
}

func TestSeedDocProcessingPolicies_NilDB(t *testing.T) {
	_, err := SeedDocProcessingPolicies(context.Background(), nil, validSeedConfig())
	if err == nil {
		t.Fatal("expected an error for a nil db")
	}
}

func TestSeedDocProcessingPolicies_InvalidConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	_, err = SeedDocProcessingPolicies(context.Background(), db, DocProcessingPolicySeedConfig{})
	if err == nil {
		t.Fatal("expected an error for an invalid config")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("no DB calls should have been made: %v", err)
	}
}

func TestSeedDocProcessingPolicies_CreatesBothPipelinesOnFirstRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	// upsertPipeline("all") and upsertPipeline("no-entities-relations") -- map
	// iteration is made deterministic by SeedDocProcessingPolicies sorting
	// names, so "all" is upserted before "no-entities-relations".
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipelines WHERE name = $1`)).
		WithArgs("all").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipelines (name, display_name, processors, legacy_equivalent)`)).
		WithArgs("all", "All", sqlmock.AnyArg(), false).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipelines WHERE name = $1`)).
		WithArgs("no-entities-relations").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipelines (name, display_name, processors, legacy_equivalent)`)).
		WithArgs("no-entities-relations", "Default", sqlmock.AnyArg(), false).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(MAX(version), 0) + 1 FROM kb.pipeline_policies`)).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipeline_policies (version, status, source_ref)`)).
		WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(30)))

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.pipeline_bindings`)).
		WithArgs(int64(1), int64(30)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.knowledge_store WHERE ks_name = $1`)).
		WithArgs("Research").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.pipeline_bindings`)).
		WithArgs(int64(7), int64(2), int64(30), "store:Research").
		WillReturnResult(sqlmock.NewResult(2, 1))

	// PolicyCompilerSQLStore.CompilePolicy (policy_compile.go's loadDefinition)
	// issues, in order: version lookup, all pipelines, this policy's
	// bindings, this policy's gates (kb.pipeline_rules where predicate IS
	// NOT NULL -- none exist, since this seed tool never writes rules), and
	// routing-clearance coverage (also none). Exact query prefixes below are
	// copied from the real queries in policy_compile.go and from the
	// existing loadBindings test's row shape (policy_compile_test.go
	// TestPolicyCompilerSQLStoreLoadBindings*, 13 columns).
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT version FROM kb.pipeline_policies WHERE id=$1`)).
		WithArgs(int64(30)).
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT name,COALESCE(display_name,''),processors,legacy_equivalent FROM kb.pipelines`)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "display_name", "processors", "legacy_equivalent"}).
			AddRow("all", "All", `{extract_metrics,extract_entity}`, false).
			AddRow("no-entities-relations", "Default", `{extract_metrics,generate_topics}`, false))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT b.id,COALESCE(b.name,''),b.priority,b.binding_kind,p.name`)).
		WithArgs(int64(30)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "priority", "kind", "pipeline", "predicate", "checksum", "active", "scope",
			"legacy_id", "doc_type", "language", "binding",
		}).
			AddRow(int64(1), "system-default", 0, "store_default", "no-entities-relations", "{}", "", true, "system", nil, "", "", "").
			AddRow(int64(2), "store:Research", 0, "store_default", "all", "{}", "", true, "knowledge_store", nil, "", "", ""))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id,name,priority,target_processor,effect,predicate::text,predicate_checksum,required_facets::text,active FROM kb.pipeline_rules`)).
		WithArgs(int64(30)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "priority", "target_processor", "effect", "predicate", "predicate_checksum", "required_facets", "active",
		}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT subject_kind,subject_id,subject_checksum FROM kb.pipeline_routing_clearance_coverage`)).
		WithArgs(int64(30), 3).
		WillReturnRows(sqlmock.NewRows([]string{"subject_kind", "subject_id", "subject_checksum"}))

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.pipeline_policies SET status = 'archived'`)).
		WithArgs(int64(30)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.pipeline_policies`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := SeedDocProcessingPolicies(context.Background(), db, validSeedConfig())
	if err != nil {
		t.Fatalf("SeedDocProcessingPolicies: %v", err)
	}
	if len(result.PipelinesCreated) != 2 {
		t.Errorf("PipelinesCreated = %v, want 2 entries", result.PipelinesCreated)
	}
	if result.PolicyID != 30 || result.PolicyVersion != 3 {
		t.Errorf("PolicyID/Version = %d/%d, want 30/3", result.PolicyID, result.PolicyVersion)
	}
	if result.BindingsWritten != 2 {
		t.Errorf("BindingsWritten = %d, want 2", result.BindingsWritten)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestSeedDocProcessingPolicies_UnknownKnowledgeStoreRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipelines WHERE name = $1`)).
		WithArgs("all").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipelines (name, display_name, processors, legacy_equivalent)`)).
		WithArgs("all", "All", sqlmock.AnyArg(), false).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipelines WHERE name = $1`)).
		WithArgs("no-entities-relations").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipelines (name, display_name, processors, legacy_equivalent)`)).
		WithArgs("no-entities-relations", "Default", sqlmock.AnyArg(), false).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(MAX(version), 0) + 1 FROM kb.pipeline_policies`)).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipeline_policies (version, status, source_ref)`)).
		WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(30)))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.pipeline_bindings`)).
		WithArgs(int64(1), int64(30)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.knowledge_store WHERE ks_name = $1`)).
		WithArgs("Research").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	_, err = SeedDocProcessingPolicies(context.Background(), db, validSeedConfig())
	if err == nil {
		t.Fatal("expected an error for an unknown knowledge store")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations (rollback not observed?): %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server/api/doc-processing && go test ./... -run TestSeedDocProcessingPolicies -v`
Expected: FAIL with `undefined: SeedDocProcessingPolicies`.

- [ ] **Step 3: Write the implementation**

```go
// policy_seed.go
package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/lib/pq"
)

// DocProcessingPolicySeedResult summarizes one SeedDocProcessingPolicies run.
type DocProcessingPolicySeedResult struct {
	PipelinesCreated []string
	PipelinesUpdated []string
	PolicyID         int64
	PolicyVersion    int
	// BindingsWritten counts the one system-wide default binding plus one
	// per cfg.Bindings entry.
	BindingsWritten int
}

// SeedDocProcessingPolicies upserts cfg's policies into kb.pipelines (by
// name), authors a new draft kb.pipeline_policies version carrying one
// system-wide store_default binding (cfg.DefaultPolicyName()) plus one
// store_default binding per cfg.Bindings entry, compiles that version, and
// activates it -- archiving whatever policy was previously active. Every
// write happens in one transaction; any failure rolls everything back and
// leaves the previously active policy untouched. Safe to call repeatedly:
// each call creates and activates a new policy version rather than mutating
// a previous one.
func SeedDocProcessingPolicies(ctx context.Context, db *sql.DB, cfg DocProcessingPolicySeedConfig) (DocProcessingPolicySeedResult, error) {
	if db == nil {
		return DocProcessingPolicySeedResult{}, errors.New("db is nil")
	}
	if err := cfg.Validate(); err != nil {
		return DocProcessingPolicySeedResult{}, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	result := DocProcessingPolicySeedResult{}
	pipelineIDs := map[string]int64{}
	policyNames := make([]string, 0, len(cfg.Policies))
	for name := range cfg.Policies {
		policyNames = append(policyNames, name)
	}
	sort.Strings(policyNames)
	for _, name := range policyNames {
		id, created, err := upsertDocProcessingPipeline(ctx, tx, name, cfg.Policies[name])
		if err != nil {
			return DocProcessingPolicySeedResult{}, fmt.Errorf("pipeline %q: %w", name, err)
		}
		pipelineIDs[name] = id
		if created {
			result.PipelinesCreated = append(result.PipelinesCreated, name)
		} else {
			result.PipelinesUpdated = append(result.PipelinesUpdated, name)
		}
	}

	var nextVersion int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM kb.pipeline_policies`).Scan(&nextVersion); err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("next policy version: %w", err)
	}
	var policyID int64
	if err := tx.QueryRowContext(ctx, `
INSERT INTO kb.pipeline_policies (version, status, source_ref)
VALUES ($1, 'draft', 'doc-processing-policy-seed')
RETURNING id`, nextVersion).Scan(&policyID); err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("create draft policy: %w", err)
	}
	result.PolicyID = policyID
	result.PolicyVersion = nextVersion

	defaultPipelineID := pipelineIDs[cfg.DefaultPolicyName()]
	if _, err := tx.ExecContext(ctx, `
INSERT INTO kb.pipeline_bindings
    (ks_store_id, pipeline_id, policy_id, name, priority, active, binding_kind)
VALUES (NULL, $1, $2, 'system-default', 0, true, 'store_default')`,
		defaultPipelineID, policyID); err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("insert system-default binding: %w", err)
	}
	result.BindingsWritten++

	storeNames := make([]string, 0, len(cfg.Bindings))
	for store := range cfg.Bindings {
		storeNames = append(storeNames, store)
	}
	sort.Strings(storeNames)
	for _, store := range storeNames {
		policyName := cfg.Bindings[store]
		var ksStoreID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM kb.knowledge_store WHERE ks_name = $1`, store).Scan(&ksStoreID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return DocProcessingPolicySeedResult{}, fmt.Errorf("binding %q: unknown knowledge store", store)
			}
			return DocProcessingPolicySeedResult{}, fmt.Errorf("binding %q: lookup knowledge store: %w", store, err)
		}
		pipelineID := pipelineIDs[policyName]
		if _, err := tx.ExecContext(ctx, `
INSERT INTO kb.pipeline_bindings
    (ks_store_id, pipeline_id, policy_id, name, priority, active, binding_kind)
VALUES ($1, $2, $3, $4, 0, true, 'store_default')`,
			ksStoreID, pipelineID, policyID, fmt.Sprintf("store:%s", store)); err != nil {
			return DocProcessingPolicySeedResult{}, fmt.Errorf("insert binding for %q: %w", store, err)
		}
		result.BindingsWritten++
	}

	compiled, err := (PolicyCompilerSQLStore{DB: tx}).CompilePolicy(ctx, policyID)
	if err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("compile policy: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE kb.pipeline_policies SET status = 'archived', modify_time = NOW() WHERE status = 'active' AND id <> $1`,
		policyID); err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("archive previous active policy: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE kb.pipeline_policies
SET status = 'active', activated_at = NOW(), activated_by = 'doc-processing-policy-seed', checksum = $1, modify_time = NOW()
WHERE id = $2`, compiled.Checksum, policyID); err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("activate policy: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

func upsertDocProcessingPipeline(ctx context.Context, tx *sql.Tx, name string, policy DocProcessingPolicySeedPolicy) (id int64, created bool, err error) {
	err = tx.QueryRowContext(ctx, `SELECT id FROM kb.pipelines WHERE name = $1`, name).Scan(&id)
	switch {
	case err == nil:
		if _, execErr := tx.ExecContext(ctx, `
UPDATE kb.pipelines SET display_name = $1, processors = $2, modify_time = NOW() WHERE id = $3`,
			policy.Description, pq.Array(policy.Processors), id); execErr != nil {
			return 0, false, execErr
		}
		return id, false, nil
	case errors.Is(err, sql.ErrNoRows):
		if scanErr := tx.QueryRowContext(ctx, `
INSERT INTO kb.pipelines (name, display_name, processors, legacy_equivalent)
VALUES ($1, $2, $3, false)
RETURNING id`, name, policy.Description, pq.Array(policy.Processors)).Scan(&id); scanErr != nil {
			return 0, false, scanErr
		}
		return id, true, nil
	default:
		return 0, false, err
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server/api/doc-processing && go test ./... -run TestSeedDocProcessingPolicies -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Run the full package test suite**

Run: `cd server/api/doc-processing && go test ./... && go vet ./...`
Expected: PASS, no new failures beyond the pre-existing, documented
`TestMetricsSQLStoreSaveMetricsPersistsMetricCategoriesEn` mock-ordering
failure (see `2026080601-handoff-keyword-step11-step12-reconciliation-and-aligns-to-term.md`
if that one is unexpectedly absent or a different failure appears).

- [ ] **Step 6: Commit**

```bash
jj commit -m "doc-processing: seed doc-processing policies into kb.pipelines/kb.pipeline_bindings"
```

---

### Task 5: CLI tool

**Files:**
- Create: `server/cmd/doc-processing-policy-seed/main.go`

**Interfaces:**
- Consumes: `docprocessing.LoadDocProcessingPolicySeedConfig`, `docprocessing.DocProcessingPolicySeedConfig.Validate`, `docprocessing.SeedDocProcessingPolicies`, `docprocessing.DocProcessingPolicySeedResult` (Tasks 2-4).
- Produces: the `doc-processing-policy-seed` binary. Nothing else depends on this package (it's `package main`).

- [ ] **Step 1: Write the CLI**

```go
// Command doc-processing-policy-seed authors Doc Processing Policies from a
// TOML config file's [doc-processing-policy-*] sections into
// kb.pipelines/kb.pipeline_bindings, then compiles and activates them as a
// new kb.pipeline_policies version -- archiving whatever was active before.
// Modeled on server/cmd/ontology-seed. See
// docs/superpowers/specs/2026-08-08-doc-processing-policy-design.md.
//
// Usage:
//
//	doc-processing-policy-seed [--config path/to/config.local.toml]
//
// Re-running after editing the config file is safe: kb.pipelines rows are
// upserted by name, and each run creates and activates a new policy
// version rather than mutating a previous one in place.
//
// The seeded policy only takes effect in an already-running doc-processor
// process after that process restarts (loadProductionPipelinePolicyState
// loads the active policy once, at process startup).
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

func main() {
	log.SetFlags(0)
	configPath := flag.String("config", "config.local.toml", "path to the TOML file holding [doc-processing-policy-*] sections")
	flag.Parse()

	cfg, err := docprocessing.LoadDocProcessingPolicySeedConfig(*configPath)
	if err != nil {
		log.Fatalf("load config %s: %v", *configPath, err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config %s: %v", *configPath, err)
	}

	db := connect()
	defer db.Close()

	result, err := docprocessing.SeedDocProcessingPolicies(context.Background(), db, cfg)
	if err != nil {
		log.Fatalf("seed: %v", err)
	}

	fmt.Printf("pipelines created: %v\n", result.PipelinesCreated)
	fmt.Printf("pipelines updated: %v\n", result.PipelinesUpdated)
	fmt.Printf("bindings written: %d\n", result.BindingsWritten)
	fmt.Printf("activated policy id=%d version=%d\n", result.PolicyID, result.PolicyVersion)
	fmt.Println("NOTE: restart the doc-processor service for this policy to take effect.")
}

func connect() *sql.DB {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "/tmp"), envOr("PG_PORT", "5432"), envOr("PG_USER", "cding"),
		envOr("PG_DB_NAME", "chenweb_test"))
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	return db
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
```

- [ ] **Step 2: Build it**

Run: `go build ./server/cmd/doc-processing-policy-seed/`
Expected: builds with no errors (note: `server/cmd/ontology-seed` has no `main_test.go` either — CLI `main` packages in this codebase are build-verified, not unit tested; Tasks 2-4 already cover all the logic this `main.go` calls).

- [ ] **Step 3: Commit**

```bash
jj commit -m "cmd: add doc-processing-policy-seed CLI"
```

---

### Task 6: Rollout verification

**Files:** none (verification only, against a real dev database).

**Interfaces:** none — this task exercises Tasks 1-5's finished artifacts end to end.

- [ ] **Step 1: Run the full workspace build/vet/test**

Run: `go build ./... && go vet ./... && go test ./server/api/doc-processing/... ./server/cmd/doc-processing-policy-seed/...`
Expected: PASS (aside from the pre-existing, documented `TestMetricsSQLStoreSaveMetricsPersistsMetricCategoriesEn` failure noted in Task 4 Step 5).

- [ ] **Step 2: Run the seed tool against a dev database**

Run: `PG_DB_NAME=chenweb_test go run ./server/cmd/doc-processing-policy-seed --config config.local.toml`
Expected: prints `pipelines created: [all no-entities-relations]`, `bindings written: 2`, and an activated policy id/version. (On a second run: `pipelines created: []`, `pipelines updated: [all no-entities-relations]`, still `bindings written: 2`, and a *new*, higher policy version — confirming Task 4's idempotency contract from the spec's §4.)

- [ ] **Step 3: Verify resolution in plan-only mode (the default) before enabling enforcement**

Per the design spec §5: with `DOC_PIPELINE_PLAN_ONLY` left unset, process one document ingested into the `Research` knowledge store and one from an unbound store, then inspect each run's persisted plan (`GET /api/v1/kb/doc-proc-plans/latest?record_id=<id>`, or query `kb.doc_process_plans` directly). Confirm the `Research` record's plan resolves `pipeline_selection` to `all` and the unbound record's resolves to `no-entities-relations` — with **no observable change** in which processors actually ran (plan-only mode never enforces).

- [ ] **Step 4: Enable enforcement and re-verify**

Set `DOC_PIPELINE_PLAN_ONLY=false` in the target environment and restart the `doc-processor` service (required per Task 5's `main.go` doc comment — the active policy loads once at process startup). Re-process the unbound-store document and confirm its actual processor run now excludes `extract_entity`/`extract_relation`, matching the `no-entities-relations` policy.

- [ ] **Step 5: Commit any documentation-impact notes**

If this rollout surfaces anything worth recording for the workspace's "what knowledge changed" protocol (per the root `CLAUDE.md`), add a short dated note to the design spec's status line and commit:

```bash
jj commit -m "docs: record doc-processing-policy rollout verification"
```

