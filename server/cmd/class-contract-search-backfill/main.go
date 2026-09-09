// Command class-contract-search-backfill populates kb.ontology_class_contract_search
// — the hybrid-search index over governed class contracts that backs the Metric
// Ontology Explorer's "Metrics of Similar Classes" node (openspec change
// analysis-node-related-metrics). It talks straight to the project database, so
// it needs no logged-in session, unlike the equivalent
// POST /api/v1/kb/ontology/class-contracts/backfill-search endpoint.
//
// Usage (run under mise so PG_* / EMBEDDING_MODEL_NAME / SEARCH_SEMANTIC_ENABLED
// are set, and from inside the repo so .models.toml resolves):
//
//	mise exec -- go run ./server/cmd/class-contract-search-backfill                 # fill missing rows
//	mise exec -- go run ./server/cmd/class-contract-search-backfill --reembed-all   # rebuild every row + embedding
//	mise exec -- go run ./server/cmd/class-contract-search-backfill --lexical-only  # skip embeddings
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/ontology/classcontractsearch"
)

func main() {
	log.SetFlags(0)
	limit := flag.Int("limit", 100000, "class terms processed per batch (default: whole corpus in one pass)")
	reembed := flag.Bool("reembed-all", false, "reindex every class term, not just the ones with no row yet")
	lexicalOnly := flag.Bool("lexical-only", false, "skip embeddings even when a model is configured (lexical tsvector only)")
	flag.Parse()

	db := connect()
	defer db.Close()
	ctx := context.Background()

	var embed classcontractsearch.EmbedFunc
	if *lexicalOnly {
		fmt.Println("mode: lexical-only (no embeddings)")
	} else {
		embed = docprocessing.EmbedSearchQuery
		fmt.Println("mode: hybrid (embeddings when SEARCH_SEMANTIC_ENABLED + EMBEDDING_MODEL_NAME are set)")
	}

	totalReindexed, totalFailed := 0, 0
	for batch := 1; ; batch++ {
		res, err := classcontractsearch.BackfillClassContractSearch(ctx, db, embed, *limit, *reembed)
		if err != nil {
			log.Fatalf("batch %d: %v", batch, err)
		}
		totalReindexed += res.Embedded
		totalFailed += res.Failed
		fmt.Printf("batch %d: scanned=%d reindexed=%d failed=%d remaining=%d\n",
			batch, res.Scanned, res.Embedded, res.Failed, res.Remaining)

		// --reembed-all does not page (it re-selects every row each call); one
		// pass with a large --limit is the intended use.
		if *reembed || res.Scanned == 0 || res.Remaining == 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Printf("done: %d class-contract rows (re)indexed, %d failed\n", totalReindexed, totalFailed)
	if totalFailed > 0 {
		os.Exit(1)
	}
}

func connect() *sql.DB {
	dbName := os.Getenv("PG_DB_NAME")
	if dbName == "" {
		log.Fatal("PG_DB_NAME must be set (mise.local.toml sets it to the service database)")
	}
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "/tmp"), envOr("PG_PORT", "5432"), postgresUserName(), dbName)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	fmt.Printf("connected to %s\n", dbName)
	return db
}

func postgresUserName() string {
	for _, key := range []string{"PG_USER_NAME", "PG_USER"} {
		if user := strings.TrimSpace(os.Getenv(key)); user != "" {
			return user
		}
	}
	return "cding"
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
