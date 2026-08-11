// Command llm-usage-report reads the local Qwen Code usage logs
// (~/.qwen/usage/token-usage-*.jsonl) and upserts per-day, per-model
// aggregates into kb.llm_usage, the daily LLM usage table shared by coding
// assistants. The deepdoc server runs the same collector on a daily schedule
// (kb.scheduled_jobs, job_type collect_llm_usage); this command is for manual
// runs and backfills.
//
// Usage:
//
//	llm-usage-report [--usage-dir ~/.qwen/usage] [--assistant qwen]
//
// Requires the same PG_* env vars as the server (PG_HOST, PG_PORT,
// PG_USER_NAME, PG_PASSWORD, PG_DB_NAME).
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/llmusage"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	usageDir := flag.String("usage-dir", defaultUsageDir(), "directory containing token-usage-*.jsonl files")
	assistant := flag.String("assistant", "qwen", "assistant name recorded in kb.llm_usage")
	flag.Parse()

	// Mirror the server: load .env from the repo root when present.
	_ = godotenv.Load()

	db := connect()
	defer db.Close()

	summary, err := llmusage.CollectAssistantUsage(context.Background(), db, *usageDir, *assistant)
	if err != nil {
		log.Fatalf("collect llm usage: %v", err)
	}
	fmt.Printf("llm-usage-report assistant=%s files=%d rows=%d requests=%d total_tokens=%d skipped=%d\n",
		*assistant, summary.Files, summary.Rows, summary.Requests, summary.TotalTokens, summary.Skipped)
}

func defaultUsageDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".qwen/usage"
	}
	return filepath.Join(home, ".qwen", "usage")
}

func connect() *sql.DB {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "127.0.0.1"),
		envOr("PG_PORT", "5432"),
		envOr("PG_USER_NAME", "cding"),
		envOr("PG_DB_NAME", "chenweb"))
	if pw := os.Getenv("PG_PASSWORD"); pw != "" {
		dsn += " password=" + pw
	}
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
