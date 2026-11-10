// Command docbuilder-classify registers a document group's documents and classifies those that
// have no category in that group with an LLM. Documents that already have a category are left
// alone, so this is run once per batch of new documents.
//
//	docbuilder-classify -group user-manuals [-model profile] [-dry-run] [-force]
//
// Needs PG_* (project database, which also holds the shared ID manager) and DOCUMENT_ROOT_DIR.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/docbuilder"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func main() {
	key := flag.String("group", "", "group key, e.g. user-manuals (required)")
	model := flag.String("model", "", "model profile in .models.toml (default $DOCBUILDER_MODEL or deepseek-flash-processor)")
	dry := flag.Bool("dry-run", false, "register documents and report what would be classified; no model call")
	force := flag.Bool("force", false, "reclassify documents that already have a category")
	flag.Parse()
	if err := run(*key, *model, *dry, *force); err != nil {
		fmt.Fprintln(os.Stderr, "docbuilder-classify:", err)
		os.Exit(1)
	}
}

func run(key, model string, dry, force bool) error {
	if key == "" {
		return fmt.Errorf("-group is required")
	}
	ctx := context.Background()
	db, err := connect()
	if err != nil {
		return err
	}
	defer db.Close()
	logger := loggerutil.CreateDefaultLogger("20261110-802")
	st, err := docbuilder.NewStoreFromEnv(ctx, db, db, "docbuilder-classify", func(f string, a ...any) { logger.Error(fmt.Sprintf(f, a...)) })
	if err != nil {
		return err
	}
	g, err := st.GroupByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("group %q: %w", key, err)
	}
	if err := st.SyncGroup(ctx, g.ID); err != nil {
		return err
	}
	entries, err := st.GroupDocs(ctx, g.ID)
	if err != nil {
		return err
	}
	pending := 0
	for _, e := range entries {
		if e.Category == "" || force {
			pending++
		}
	}
	fmt.Printf("group %s: %d documents, %d to classify\n", key, len(entries), pending)
	if dry || pending == 0 {
		return nil
	}
	cl, modelName, err := docbuilder.NewClassifierFromEnv(model, logger)
	if err != nil {
		return err
	}
	n, err := st.ClassifyGroup(ctx, g, cl, modelName, force)
	fmt.Printf("classified %d documents\n", n)
	return err
}

func connect() (*sql.DB, error) {
	name := os.Getenv("PG_DB_NAME")
	if name == "" {
		return nil, fmt.Errorf("PG_DB_NAME must be set")
	}
	user := firstEnv("PG_USER_NAME", "PG_USER")
	if user == "" {
		user = "cding"
	}
	host := os.Getenv("PG_HOST")
	if host == "" {
		host = "/tmp"
	}
	port := os.Getenv("PG_PORT")
	if port == "" {
		port = "5432"
	}
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable", host, port, user, name)
	if pw := os.Getenv("PG_PASSWORD"); pw != "" {
		dsn += " password=" + pw
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	return db, db.Ping()
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
