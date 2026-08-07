package docprocessing

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
	"github.com/chendingplano/deepdoc/server/api/ontology/modules"
	"github.com/chendingplano/deepdoc/server/api/ontology/names"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
	_ "github.com/lib/pq"
)

func TestKeywordResolutionPersistsGovernedMetricIDsInPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	t.Setenv("KEYWORD_RESOLVER_MODE", "observe")

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping test database: %v", err)
	}

	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	moduleID := "i2_metric_" + suffix
	termID := "i2:metric_" + suffix
	conceptID := "kwc_i2_" + suffix
	termLabel := "I2 Luminance " + suffix
	exactSurface := "I2 display luminance " + suffix
	artifactID := "i2:metric:" + suffix
	metricID := "i2-metric-" + suffix
	inputRecordID := time.Now().UnixNano()

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		statements := []struct {
			query string
			args  []any
		}{
			{`DELETE FROM kb.metrics WHERE input_record_id = $1`, []any{inputRecordID}},
			{`DELETE FROM kb.keyword_occurrences WHERE artifact_id = $1`, []any{artifactID}},
			{`DELETE FROM kb.semantic_assertions WHERE subject_ref_kind = 'keyword_concept' AND subject_ref_id = $1`, []any{conceptID}},
			{`DELETE FROM kb.semid_decision_log WHERE (input->>'surface') = $1 OR (input->>'concept_id') = $2`, []any{exactSurface, conceptID}},
			{`DELETE FROM kb.keyword_surfaces WHERE concept_id = $1`, []any{conceptID}},
			{`DELETE FROM kb.keyword_concepts WHERE concept_id = $1`, []any{conceptID}},
			{`DELETE FROM kb.ontology_term_labels WHERE term_id = $1`, []any{termID}},
			{`DELETE FROM kb.ontology_terms WHERE term_id = $1`, []any{termID}},
			{`DELETE FROM kb.ontology_module_releases WHERE module_id = $1`, []any{moduleID}},
			{`DELETE FROM kb.ontology_modules WHERE module_id = $1`, []any{moduleID}},
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(cleanupCtx, statement.query, statement.args...); err != nil {
				t.Logf("cleanup %q: %v", strings.Fields(statement.query)[2], err)
			}
		}
	})

	if _, err := (modules.ModuleStore{DB: db}).CreateModule(ctx, modules.Module{
		ModuleID: moduleID,
		Title:    "I2 metric integration fixture",
		Status:   "draft",
		CreateBy: "integration-test",
		ModifyBy: "integration-test",
	}); err != nil {
		t.Fatalf("create fixture module: %v", err)
	}
	if _, err := (terms.TermStore{DB: db}).CreateTerm(ctx, terms.Term{
		TermID:     termID,
		TermKind:   "metric_definition",
		ModuleID:   moduleID,
		Status:     "approved",
		Definition: "Isolated metric definition for the I2 PostgreSQL integration test.",
		Scope:      "_",
		CreateBy:   "integration-test",
		ModifyBy:   "integration-test",
	}); err != nil {
		t.Fatalf("create fixture term: %v", err)
	}
	if _, err := (terms.LabelStore{DB: db}).CreateLabel(ctx, terms.TermLabel{
		TermID:    termID,
		Label:     termLabel,
		Lang:      "en",
		LabelRole: "prefLabel",
		Status:    "approved",
		CreateBy:  "integration-test",
		ModifyBy:  "integration-test",
	}); err != nil {
		t.Fatalf("create fixture term label: %v", err)
	}
	if _, err := (modules.ReleaseStore{DB: db}).CreateRelease(ctx, moduleID, "1.0.0", "integration-test"); err != nil {
		t.Fatalf("release fixture metric definition: %v", err)
	}

	conceptStore := keywords.ConceptStore{DB: db}
	if _, err := conceptStore.CreateConcept(ctx, keywords.Concept{
		ConceptID:   conceptID,
		PrefLabel:   termLabel,
		Scope:       "_",
		Status:      "active",
		GlossSource: "integration-test",
	}); err != nil {
		t.Fatalf("create fixture keyword concept: %v", err)
	}
	surfaceStore := keywords.SurfaceStore{DB: db}
	for _, surface := range []keywords.Surface{
		{ConceptID: conceptID, Surface: termLabel, LabelRole: "pref", AliasType: "pref", Lang: "en", Scope: "_", Provenance: "integration-test"},
		{ConceptID: conceptID, Surface: exactSurface, LabelRole: "alt", AliasType: "synonym", Lang: "en", Scope: "_", Provenance: "integration-test"},
	} {
		if _, err := surfaceStore.CreateSurface(ctx, surface); err != nil {
			t.Fatalf("create fixture keyword surface %q: %v", surface.Surface, err)
		}
	}

	alignments := &keywords.AlignmentsStore{
		Assertions:  assertions.AssertionStore{DB: db},
		DecisionLog: semid.DecisionLogStore{DB: db},
		Scope:       "_",
	}
	resolver := names.NewResolverWithAlignments(
		&keywords.KeywordFamily{DB: db, ResolverMode: os.Getenv("KEYWORD_RESOLVER_MODE")},
		alignments,
	)
	resolution, err := resolver.ResolveAndObserve(ctx,
		names.ResolveNameRequest{Name: exactSurface, Scope: "_"},
		names.NameOccurrence{
			ArtifactType: "metric",
			ArtifactID:   artifactID,
			FieldPath:    "metric_name",
			Context:      "A real document sentence reports " + exactSurface + ".",
		},
	)
	if err != nil {
		t.Fatalf("resolve and observe exact fixture surface: %v", err)
	}
	if resolution.Status != names.StatusTermResolved || resolution.ConceptID != conceptID || resolution.TermID != termID {
		t.Fatalf("resolution = %#v, want term_resolved concept %q term %q", resolution, conceptID, termID)
	}

	var occurrenceConceptID, occurrenceTermID, occurrenceStatus string
	if err := db.QueryRowContext(ctx, `
SELECT concept_id, term_id, resolution_status
FROM kb.keyword_occurrences
WHERE artifact_type = 'metric' AND artifact_id = $1 AND field_path = 'metric_name'`, artifactID).
		Scan(&occurrenceConceptID, &occurrenceTermID, &occurrenceStatus); err != nil {
		t.Fatalf("query observed occurrence: %v", err)
	}
	if occurrenceConceptID != conceptID || occurrenceTermID != termID || occurrenceStatus != "term_resolved" {
		t.Fatalf("occurrence = (%q, %q, %q), want (%q, %q, term_resolved)", occurrenceConceptID, occurrenceTermID, occurrenceStatus, conceptID, termID)
	}

	var alignmentCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*)
FROM kb.semantic_assertions
WHERE subject_ref_kind = 'keyword_concept' AND subject_ref_id = $1
  AND predicate_term_id = 'core:aligns_to_term'
  AND object_ref_kind = 'ontology_term' AND object_ref_id = $2
  AND status = 'accepted'`, conceptID, termID).Scan(&alignmentCount); err != nil {
		t.Fatalf("query accepted alignment: %v", err)
	}
	if alignmentCount != 1 {
		t.Fatalf("accepted aligns_to_term assertion count = %d, want 1", alignmentCount)
	}

	metricsStore := &ResolvingMetricsStore{
		Inner:    MetricsSQLStore{DB: db},
		Resolver: resolver,
	}
	inserted, err := metricsStore.SaveMetrics(ctx, SaveMetricsRequest{
		InputRecordID: inputRecordID,
		Language:      "en",
		ModelName:     "integration-test",
		PromptName:    "integration-test",
		Metrics: []map[string]any{{
			"metric_id":   metricID,
			"metric_name": exactSurface,
		}},
	})
	if err != nil {
		t.Fatalf("persist metric through resolving SQL store: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("inserted metrics = %d, want 1", inserted)
	}

	var persistedConceptID, persistedTermID string
	if err := db.QueryRowContext(ctx, `
SELECT keyword_concept_id, metric_definition_term_id
FROM kb.metrics
WHERE input_record_id = $1 AND metric_id = $2`, inputRecordID, metricID).
		Scan(&persistedConceptID, &persistedTermID); err != nil {
		t.Fatalf("query persisted metric identities: %v", err)
	}
	if persistedConceptID != conceptID || persistedTermID != termID {
		t.Fatalf("persisted metric identities = (%q, %q), want (%q, %q)", persistedConceptID, persistedTermID, conceptID, termID)
	}
}
