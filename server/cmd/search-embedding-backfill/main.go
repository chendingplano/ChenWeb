// Command search-embedding-backfill populates kb.search_artifacts.embedding with
// the configured embedding model (EMBEDDING_MODEL_NAME). It talks straight to the
// project database, so it needs no logged-in session, unlike the equivalent
// POST /api/v1/kb/search/backfill-embeddings endpoint, and it embeds through the
// indexing pipeline's batched, concurrent, rate-limited path rather than one text
// per request. Its main use is after an embedding-model switch (e.g. migration
// 20260925000004 reset every vector to NULL when moving to bge-m3 / 1024 dims).
//
// entity and relation rows are skipped when EMBED_ENTITY_RELATION is off,
// matching what the indexing pipeline writes.
//
// Usage (run under mise so PG_* / EMBEDDING_* are set, and from inside the repo
// so .models.toml resolves):
//
//	mise exec -- go run ./server/cmd/search-embedding-backfill                          # fill NULL embeddings, all types
//	mise exec -- go run ./server/cmd/search-embedding-backfill --artifact-type metric   # one type
//	mise exec -- go run ./server/cmd/search-embedding-backfill --reembed-all            # recompute every row
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

	"github.com/chendingplano/shared/go/api/loggerutil"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/kbsearch"
)

const updateSQL = `UPDATE kb.search_artifacts
SET embedding = $1::vector, embedding_text = $2, updated_at = NOW()
WHERE artifact_type = $3 AND artifact_id = $4`

func main() {
	log.SetFlags(0)
	artifactType := flag.String("artifact-type", "all", "artifact type to backfill (\"all\" = every type)")
	batch := flag.Int("batch", 400, "rows selected and embedded per round")
	reembed := flag.Bool("reembed-all", false, "recompute every row, not just NULL embeddings")
	flag.Parse()

	logger := loggerutil.CreateDefaultLogger("20260925-417")
	db := connect()
	defer db.Close()
	ctx := context.Background()

	types, err := resolveTypes(ctx, db, *artifactType)
	if err != nil {
		log.Fatalf("resolve artifact types: %v", err)
	}
	fmt.Printf("model=%s dim=%d types=%v\n", os.Getenv("EMBEDDING_MODEL_NAME"), kbsearch.ConfiguredEmbeddingDim(), types)
	logger.Info("search embedding backfill started", "types", strings.Join(types, ","), "reembed_all", *reembed, "batch", *batch)

	totalEmbedded, totalFailed := 0, 0
	for _, at := range types {
		// Keyset pagination on artifact_id: failed rows stay NULL but are never
		// re-selected in this run, and --reembed-all walks the partition once.
		lastID := ""
		for round := 1; ; round++ {
			start := time.Now()
			rows, err := selectCandidates(ctx, db, at, lastID, *reembed, *batch)
			if err != nil {
				log.Fatalf("[%s] select: %v", at, err)
			}
			if len(rows) == 0 {
				break
			}
			lastID = rows[len(rows)-1].ArtifactID

			docprocessing.EmbedRegistryRows(ctx, rows, logger, "backfill search embeddings", "20260925-418")

			embedded, failed := 0, 0
			for _, r := range rows {
				if len(r.Embedding) != kbsearch.ConfiguredEmbeddingDim() {
					failed++
					continue
				}
				if _, err := db.ExecContext(ctx, updateSQL, kbsearch.FormatVectorLiteral(r.Embedding), r.EmbeddingText, r.ArtifactType, r.ArtifactID); err != nil {
					log.Fatalf("[%s] update %s: %v", at, r.ArtifactID, err)
				}
				embedded++
			}
			totalEmbedded += embedded
			totalFailed += failed
			fmt.Printf("[%s] round %d: selected=%d embedded=%d failed=%d (%s)\n",
				at, round, len(rows), embedded, failed, time.Since(start).Round(time.Second))
			if embedded == 0 {
				log.Printf("[%s] whole round failed; stopping this type (model unreachable?)", at)
				break
			}
		}
	}
	logger.Info("search embedding backfill finished", "embedded", totalEmbedded, "failed", totalFailed)
	fmt.Printf("done: %d rows embedded, %d failed\n", totalEmbedded, totalFailed)
	if totalFailed > 0 {
		os.Exit(1)
	}
}

func selectCandidates(ctx context.Context, db *sql.DB, artifactType, afterID string, reembed bool, limit int) ([]kbsearch.RegistryRow, error) {
	q := `SELECT artifact_id, COALESCE(NULLIF(embedding_text, ''), search_document)
FROM kb.search_artifacts
WHERE artifact_type = $1 AND artifact_id > $2
  AND COALESCE(NULLIF(embedding_text, ''), NULLIF(search_document, '')) IS NOT NULL`
	if !reembed {
		q += " AND embedding IS NULL"
	}
	q += " ORDER BY artifact_id LIMIT $3"
	rs, err := db.QueryContext(ctx, q, artifactType, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []kbsearch.RegistryRow
	for rs.Next() {
		r := kbsearch.RegistryRow{ArtifactType: artifactType}
		if err := rs.Scan(&r.ArtifactID, &r.EmbeddingText); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rs.Err()
}

// resolveTypes expands "all" to the artifact types present in the table, minus
// entity/relation when EMBED_ENTITY_RELATION is off.
func resolveTypes(ctx context.Context, db *sql.DB, want string) ([]string, error) {
	if want = strings.TrimSpace(want); want != "" && want != "all" {
		return []string{want}, nil
	}
	rows, err := db.QueryContext(ctx, "SELECT DISTINCT artifact_type FROM kb.search_artifacts ORDER BY 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var at string
		if err := rows.Scan(&at); err != nil {
			return nil, err
		}
		if !kbsearch.EmbedEntityRelationEnabled() && (at == "entity" || at == "relation") {
			fmt.Printf("skipping %s (EMBED_ENTITY_RELATION is off)\n", at)
			continue
		}
		types = append(types, at)
	}
	return types, rows.Err()
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
