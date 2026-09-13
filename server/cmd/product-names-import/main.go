// Command product-names-import loads China's NMPA medical-device
// classification catalog (医疗器械分类目录) into kb.product_names. Each
// source row is a product category whose 品名举例 cell packs several
// example product names into one 、/，-delimited string; this command
// explodes that into one kb.product_names row per example name, carrying
// its category context along.
//
// It also (a) LLM-translates each unique Chinese name to English, since the
// source file has Chinese names only, and (b) computes the shared
// keyword-canonicalization normalizer's derived keys (tiers 0-4 of
// doc-2026080403's tier ladder) for both the Chinese and English forms,
// storing them in the `keywords` JSONB column. It does NOT call the
// resolver or write to kb.keyword_surfaces/kb.keyword_concepts — mapping
// each name to a keyword_concept_id is deliberately deferred.
//
// Usage:
//
//	product-names-import --xlsx <medical-product-names.xlsx> [--source <id>]
//	    [--translate-model-ref <ref>] [--translate-batch-size 40]
//	    [--skip-translate] [--dry-run]
//
// Re-running is safe: rows are scoped by --source and fully replaced
// (DELETE then re-insert) each run.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

const defaultSource = "cn_nmpa_medical_device_classification_catalog"

func main() {
	log.SetFlags(0)
	xlsxPath := flag.String("xlsx", "", "path to medical-product-names.xlsx (required)")
	source := flag.String("source", defaultSource, "source identifier; existing rows with this source are replaced")
	translateModelRef := flag.String("translate-model-ref", envOr("TRANSLATION_MODEL_NAME", "deepseek-flash-chen"), "model ref (key in MODEL_DEF_FILE) for name translation")
	translateBatchSize := flag.Int("translate-batch-size", 40, "names per translation LLM call")
	skipTranslate := flag.Bool("skip-translate", false, "skip LLM translation; product_name_en stays empty")
	dryRun := flag.Bool("dry-run", false, "parse and translate but do not write to the database")
	flag.Parse()

	if strings.TrimSpace(*xlsxPath) == "" {
		log.Fatal("--xlsx is required")
	}

	catalogRows, err := readCatalogRows(*xlsxPath)
	if err != nil {
		log.Fatalf("read xlsx: %v", err)
	}
	fmt.Printf("parsed %d category rows from %s\n", len(catalogRows), *xlsxPath)

	rows := explodeAll(catalogRows)
	fmt.Printf("exploded into %d product-name rows\n", len(rows))

	ctx := context.Background()
	translations := map[string]string{}
	if !*skipTranslate {
		translations = translateUniqueNames(ctx, rows, *translateModelRef, *translateBatchSize)
	}

	for i := range rows {
		rows[i].ProductNameEN = translations[rows[i].ProductName]
	}

	if *dryRun {
		translated := 0
		for _, r := range rows {
			if r.ProductNameEN != "" {
				translated++
			}
		}
		fmt.Printf("dry-run: %d rows ready (%d translated), no database write\n", len(rows), translated)
		return
	}

	db := connect()
	defer db.Close()

	if err := insertRows(ctx, db, *source, rows); err != nil {
		log.Fatalf("insert: %v", err)
	}
	fmt.Printf("imported %d rows into kb.product_names (source=%s)\n", len(rows), *source)
}

// translateUniqueNames translates each distinct Chinese product name once
// and returns a zh -> en lookup, so a name repeated across many categories
// only costs one LLM call.
func translateUniqueNames(ctx context.Context, rows []productNameRow, modelRef string, batchSize int) map[string]string {
	seen := make(map[string]bool)
	unique := make([]string, 0, len(rows))
	for _, r := range rows {
		if !seen[r.ProductName] {
			seen[r.ProductName] = true
			unique = append(unique, r.ProductName)
		}
	}
	fmt.Printf("translating %d unique names (model ref %q)...\n", len(unique), modelRef)

	t, err := newTranslator(modelRef, batchSize)
	if err != nil {
		fmt.Printf("  warning: translator unavailable (%v); continuing with product_name_en empty\n", err)
		return map[string]string{}
	}
	return t.translateAll(ctx, unique)
}

// insertRows replaces every kb.product_names row for `source` with `rows`,
// in one transaction.
func insertRows(ctx context.Context, db *sql.DB, source string, rows []productNameRow) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.product_names WHERE source = $1`, source); err != nil {
		return fmt.Errorf("delete existing rows for source %q: %w", source, err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO kb.product_names (
			seq_no, sub_catalog, category_l1, category_l2, description,
			intended_use, product_name, product_name_en, regulatory_class,
			keywords, source, extra_info, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'approved')
	`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for i, r := range rows {
		kw, err := json.Marshal(buildKeywords(r.ProductName, r.ProductNameEN))
		if err != nil {
			return fmt.Errorf("marshal keywords for row %d: %w", i, err)
		}
		extra, err := json.Marshal(map[string]any{"excel_row": r.ExcelRow})
		if err != nil {
			return fmt.Errorf("marshal extra_info for row %d: %w", i, err)
		}
		if _, err := stmt.ExecContext(ctx,
			r.SeqNo, r.SubCatalog, r.CategoryL1, r.CategoryL2, r.Description,
			r.IntendedUse, r.ProductName, r.ProductNameEN, r.RegulatoryClass,
			kw, source, extra,
		); err != nil {
			return fmt.Errorf("insert row %d (%q): %w", i, r.ProductName, err)
		}
		if (i+1)%1000 == 0 {
			fmt.Printf("  inserted %d/%d rows\n", i+1, len(rows))
		}
	}

	return tx.Commit()
}

func connect() *sql.DB {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "/tmp"), envOr("PG_PORT", "5432"), postgresUserName(),
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

func postgresUserName() string {
	for _, key := range []string{"PG_USER_NAME", "PG_USER"} {
		if user := strings.TrimSpace(os.Getenv(key)); user != "" {
			return user
		}
	}
	return "cding"
}

func envOr(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}
