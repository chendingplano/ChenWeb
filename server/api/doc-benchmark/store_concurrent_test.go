package docbenchmark

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

func TestConcurrentClaim(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	_, file, _, _ := runtime.Caller(0)
	migrations := filepath.Join(filepath.Dir(file), "../../../project_migrations")
	bootstrap, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Close()
	if err := bootstrap.Ping(); err != nil {
		t.Fatal(err)
	}
	goose.SetDialect("postgres")
	if err := goose.Up(bootstrap, migrations); err != nil {
		t.Fatal(err)
	}

	// Every fixture key is unique, so cleanup can only touch rows created by
	// this test even when it runs against a shared development database.
	key := fmt.Sprintf("concurrent-claim-%d", time.Now().UnixNano())
	var experimentID, runID, caseRunID string
	err = bootstrap.QueryRow(`INSERT INTO kb.benchmark_experiments
		(name,dataset_id,dataset_version,dataset_hash,raw_request_toml,raw_request_hash,
		 resolved_experiment_json,resolved_file_hashes_json,resolved_case_set_json)
		VALUES ($1,'test','v1',$2,$3,$4,'{}','{}','{}') RETURNING id`,
		key, key+"-dataset", "{}", key+"-request").Scan(&experimentID)
	if err != nil {
		t.Fatal(err)
	}
	err = bootstrap.QueryRow(`INSERT INTO kb.benchmark_runs
		(experiment_id,variant_name,requested_json,resolved_json,config_json,prompt_json,scorer_json,pricing_json)
		VALUES ($1,$2,'{}','{}','{}','{}','{}','{}') RETURNING id`, experimentID, key).Scan(&runID)
	if err != nil {
		t.Fatal(err)
	}
	err = bootstrap.QueryRow(`INSERT INTO kb.benchmark_case_runs
		(run_id,case_id,repetition,applicability,tags_json,lifecycle)
		VALUES ($1,$2,1,'applicable','{}','pending') RETURNING id`, runID, key).Scan(&caseRunID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Child rows must be removed before their parents. Ignore cleanup errors
		// so a failed assertion does not hide its original diagnostic.
		_, _ = bootstrap.Exec(`DELETE FROM kb.benchmark_case_attempts WHERE case_run_id=$1`, caseRunID)
		_, _ = bootstrap.Exec(`DELETE FROM kb.benchmark_case_runs WHERE id=$1`, caseRunID)
		_, _ = bootstrap.Exec(`DELETE FROM kb.benchmark_runs WHERE id=$1`, runID)
		_, _ = bootstrap.Exec(`DELETE FROM kb.benchmark_experiments WHERE id=$1`, experimentID)
	})

	first, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	second, err := sql.Open("postgres", dsn)
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	first.SetMaxOpenConns(1)
	second.SetMaxOpenConns(1)
	t.Cleanup(func() { first.Close(); second.Close() })
	if err := first.Ping(); err != nil {
		t.Fatal(err)
	}
	if err := second.Ping(); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan Claim, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, db := range []*sql.DB{first, second} {
		wg.Add(1)
		go func(i int, db *sql.DB) {
			defer wg.Done()
			<-start
			claim, err := (SQLStore{DB: db}).ClaimAttempt(context.Background(), caseRunID, fmt.Sprintf("owner-%d", i), time.Now().UTC(), time.Minute, 3)
			if err != nil {
				errs <- err
				return
			}
			results <- claim
		}(i, db)
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var claims []Claim
	for claim := range results {
		claims = append(claims, claim)
	}
	if len(claims) != 2 {
		t.Fatalf("got %d claim results, want 2", len(claims))
	}
	winners := 0
	for _, claim := range claims {
		if claim.Claimed {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("claimed results=%d, want exactly one winner", winners)
	}
	var attempts int
	if err := bootstrap.QueryRow(`SELECT count(*) FROM kb.benchmark_case_attempts WHERE case_run_id=$1`, caseRunID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatalf("attempt rows=%d, want 1", attempts)
	}
}
