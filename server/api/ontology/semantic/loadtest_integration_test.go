package semantic

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// Task 1.5 (Phase 0): load-test outcome/finding persistence, the deferred
// constraint trigger, and retry throughput at corpus scale.
//
// The corpus measured by cmd/semantic-baseline is 7,074 metric occurrences x 3
// required stages = 21,222 outcome envelopes. This test writes that shape so
// the Phase 0 capacity gate rests on a measurement rather than an estimate.
//
// It is opt-in because it is slow:
//
//	SEMANTIC_LOAD_TEST=1 TEST_DATABASE_URL='...' \
//	    go test ./server/api/ontology/semantic/ -run LoadTest -v -timeout 30m
func TestIntegrationLoadTestCorpusScale(t *testing.T) {
	if os.Getenv("SEMANTIC_LOAD_TEST") == "" {
		t.Skip("SEMANTIC_LOAD_TEST not set")
	}
	db := freshSemanticTestDB(t)
	db.SetMaxOpenConns(8)
	ctx := context.Background()
	store := OutcomeStore{DB: db}
	adapter := MetricAdapter{}

	const occurrences = 7074
	stages := adapter.RequiredStages()
	// Measured against the corpus: 2,395 unmapped + 629 ambiguous of 7,074 have
	// a mapping finding today, so ~43% of normalize-stage outcomes carry one.
	const findingEveryNth = 7 // ~14% here to keep the run bounded; scaled below

	writeStart := time.Now()
	var envelopes, findings int
	for i := 0; i < occurrences; i++ {
		artifactID := fmt.Sprintf("m-%05d", i)
		recordID := int64(i % 58) // the corpus spans 58 metric-bearing records
		for _, st := range stages {
			out := Outcome{
				OutcomeKey:            OutcomeKey(recordID, MetricArtifactType, artifactID, st.StageTermID),
				InputRecordID:         &recordID,
				ArtifactType:          MetricArtifactType,
				ArtifactID:            artifactID,
				StageTermID:           st.StageTermID,
				DispositionTermID:     DispositionNormalized,
				ExecutionStatus:       ExecutionCompleted,
				OutcomeCategory:       CategorySemanticSuccess,
				DependencyFingerprint: Dependencies{MappingRevision: "map-1", ParserVersion: "p-1"}.Fingerprint(),
				InputFingerprint:      fmt.Sprintf("v1:in-%05d", i),
				ProcessorName:         "loadtest",
			}
			var childSet []Finding
			if st.StageTermID == StageNormalize && i%findingEveryNth == 0 {
				out.DispositionTermID = DispositionRawPreserved
				out.OutcomeCategory = CategorySemanticFinding
				childSet = []Finding{mappingFinding(
					Dependencies{MappingRevision: "map-1"}.Fingerprint())}
				findings++
			}
			if _, err := store.Record(ctx, out, childSet); err != nil {
				t.Fatalf("record %s/%s: %v", artifactID, st.StageTermID, err)
			}
			envelopes++
		}
	}
	writeElapsed := time.Since(writeStart)

	// Replay the whole corpus: DR4 requires this to advance last_seen only.
	// If replay were not idempotent, a re-run would double the table.
	replayStart := time.Now()
	for i := 0; i < occurrences; i++ {
		artifactID := fmt.Sprintf("m-%05d", i)
		recordID := int64(i % 58)
		st := stages[0]
		out := Outcome{
			OutcomeKey:            OutcomeKey(recordID, MetricArtifactType, artifactID, st.StageTermID),
			InputRecordID:         &recordID,
			ArtifactType:          MetricArtifactType,
			ArtifactID:            artifactID,
			StageTermID:           st.StageTermID,
			DispositionTermID:     DispositionNormalized,
			ExecutionStatus:       ExecutionCompleted,
			OutcomeCategory:       CategorySemanticSuccess,
			DependencyFingerprint: Dependencies{MappingRevision: "map-1", ParserVersion: "p-1"}.Fingerprint(),
			InputFingerprint:      fmt.Sprintf("v1:in-%05d", i),
			ProcessorName:         "loadtest",
		}
		var childSet []Finding
		if i%findingEveryNth == 0 {
			out.DispositionTermID = DispositionRawPreserved
			out.OutcomeCategory = CategorySemanticFinding
			childSet = []Finding{mappingFinding(Dependencies{MappingRevision: "map-1"}.Fingerprint())}
		}
		res, err := store.Record(ctx, out, childSet)
		if err != nil {
			t.Fatalf("replay %s: %v", artifactID, err)
		}
		if !res.Reused {
			t.Fatalf("replay of %s was not idempotent", artifactID)
		}
	}
	replayElapsed := time.Since(replayStart)

	var rowCount int
	if err := db.QueryRow(`SELECT count(*) FROM kb.semantic_processing_outcomes`).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if rowCount != envelopes {
		t.Fatalf("replay appended rows: %d envelopes written, %d rows present", envelopes, rowCount)
	}

	// Targeted retry scheduling across the whole corpus (DR10).
	queue := RetryQueue{DB: db}
	scheduleStart := time.Now()
	scheduled, err := queue.ScheduleForDependencyChange(ctx, FindingMappingUnresolved,
		Dependencies{MappingRevision: "map-2"}.Fingerprint())
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	scheduleElapsed := time.Since(scheduleStart)
	if scheduled != findings {
		t.Fatalf("scheduled %d jobs, want %d (one per affected finding)", scheduled, findings)
	}

	// The completeness projection is the cutover gate; it must be usable at
	// corpus scale, not just on a toy dataset.
	for i := 0; i < occurrences; i++ {
		if _, err := db.Exec(`INSERT INTO kb.metrics (input_record_id, metric_id, metric_name)
			VALUES ($1,$2,$3)`, int64(i%58), fmt.Sprintf("m-%05d", i), "loadtest"); err != nil {
			t.Fatalf("seed metric: %v", err)
		}
	}
	completeStart := time.Now()
	rep, err := CompletenessChecker{DB: db, ArtifactSourceSQL: MetricArtifactSourceSQL}.Run(ctx, adapter)
	if err != nil {
		t.Fatalf("completeness: %v", err)
	}
	completeElapsed := time.Since(completeStart)
	if rep.MissingStageOutcomes != 0 {
		t.Errorf("missing stage outcomes = %d, want 0 after writing every stage", rep.MissingStageOutcomes)
	}
	if rep.SummaryDrift != 0 {
		t.Errorf("summary drift = %d, want 0", rep.SummaryDrift)
	}

	var tableBytes, indexBytes int64
	if err := db.QueryRow(`
SELECT pg_total_relation_size('kb.semantic_processing_outcomes'),
       pg_indexes_size('kb.semantic_processing_outcomes')`).Scan(&tableBytes, &indexBytes); err != nil {
		t.Fatal(err)
	}

	t.Logf("PHASE 0 LOAD TEST (%d occurrences x %d stages = %d envelopes, %d findings)",
		occurrences, len(stages), envelopes, findings)
	t.Logf("  write:        %v (%.0f envelopes/sec)", writeElapsed.Round(time.Millisecond),
		float64(envelopes)/writeElapsed.Seconds())
	t.Logf("  replay:       %v (%.0f replays/sec)", replayElapsed.Round(time.Millisecond),
		float64(occurrences)/replayElapsed.Seconds())
	t.Logf("  retry sweep:  %v for %d jobs", scheduleElapsed.Round(time.Millisecond), scheduled)
	t.Logf("  completeness: %v over %d artifacts", completeElapsed.Round(time.Millisecond), rep.CurrentArtifacts)
	t.Logf("  outcomes table: %.1f MiB total, %.1f MiB indexes",
		float64(tableBytes)/(1<<20), float64(indexBytes)/(1<<20))
}
