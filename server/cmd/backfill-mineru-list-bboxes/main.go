// Command backfill-mineru-list-bboxes is a one-off repair tool.
//
// Background: MinerU's content_list.json collapses an entire list block into
// a single shared bbox plus a list_items string array. Before the fix in
// ChenWeb/server/api/file-converters/mineru.go, every exploded list-item row
// in the canonical line file (<record>_mineru.txt) reused that one shared
// (too-large) bbox, so highlighting a single cited line visually painted the
// whole list.
//
// New parses are fixed at the source: parser_mineru.py's
// annotate_list_item_bboxes() derives one bbox per list item from MinerU's
// finer-grained *_middle.json, rescaled into content_list.json's coordinate
// space (the two files use different coordinate systems for the same page),
// and stores it as list_item_bboxes on the consolidated JSON
// (kb.inputs.result_filename).
//
// This tool repairs already-parsed records' canonical .txt line files with
// that same, already-computed data — it does not redo the middle.json
// matching/rescaling itself (see backfill_list_item_bboxes.py, which must be
// run first to populate list_item_bboxes on records parsed before the fix).
// It locates each list's lines in .txt by matching the list's content
// sequence, not by line_number or by detecting duplicate bboxes: structure
// analysis can merge or remove earlier lines (see
// KnowledgeStore/Capsules/coding-capsules/doc-processor/structure-analyzer-static-spec.md
// §1.6.1, §1.6.2, §1.6.7), which permanently shifts every later line_number,
// and a first (buggy) run of this tool can already have made every bbox
// distinct-but-wrong, destroying the duplicate-bbox signal. Content is never
// altered by those stages, so matching on it is robust to both. This makes
// every run of this tool idempotent.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/pathutil"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type inputRecord struct {
	ID             int64
	ResultFilename string
}

type mineruDoc struct {
	Pages []mineruPage `json:"pages"`
}

type mineruPage struct {
	PageNumber int          `json:"page_number"`
	Items      []mineruItem `json:"items"`
}

type mineruItem struct {
	Type           string      `json:"type"`
	ListItems      []string    `json:"list_items"`
	ListItemBBoxes [][]float64 `json:"list_item_bboxes"`
}

func main() {
	dryRun := flag.Bool("dry-run", false, "report planned changes without writing files")
	onlyRecordID := flag.Int64("record-id", 0, "only process this kb.inputs.id (0 = all mineru records)")
	flag.Parse()

	_ = godotenv.Load()

	db, err := connectDB()
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer db.Close()

	records, err := fetchMineruRecords(db, *onlyRecordID)
	if err != nil {
		log.Fatalf("fetch records: %v", err)
	}
	log.Printf("found %d mineru-parsed record(s) to check", len(records))

	patchedRecords, totalLinesFixed, skipped := 0, 0, 0
	for _, rec := range records {
		fixed, err := backfillRecord(rec, *dryRun)
		if err != nil {
			log.Printf("record %d: skipped: %v", rec.ID, err)
			skipped++
			continue
		}
		if fixed > 0 {
			patchedRecords++
			totalLinesFixed += fixed
			verb := "would fix"
			if !*dryRun {
				verb = "fixed"
			}
			log.Printf("record %d: %s %d line(s)", rec.ID, verb, fixed)
		}
	}
	log.Printf(
		"done: %d/%d record(s) patched, %d line(s) fixed, %d record(s) skipped (dry-run=%v)",
		patchedRecords, len(records), totalLinesFixed, skipped, *dryRun,
	)
}

func connectDB() (*sql.DB, error) {
	host := envOr("PG_HOST", "localhost")
	port := envOr("PG_PORT", "5432")
	user := envOr("PG_USER_NAME", "")
	dbname := envOr("PG_DB_NAME", "")
	password := os.Getenv("PG_PASSWORD")

	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable", host, port, user, dbname)
	if password != "" {
		dsn += " password=" + password
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func fetchMineruRecords(db *sql.DB, onlyID int64) ([]inputRecord, error) {
	var rows *sql.Rows
	var err error
	if onlyID > 0 {
		rows, err = db.Query(
			`SELECT id, result_filename FROM kb.inputs WHERE id = $1 AND parser_name = 'mineru'`,
			onlyID,
		)
	} else {
		rows, err = db.Query(
			`SELECT id, result_filename FROM kb.inputs
			 WHERE type = 'pdf' AND parser_name = 'mineru'
			   AND result_filename IS NOT NULL AND result_filename <> ''
			 ORDER BY id`,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []inputRecord
	for rows.Next() {
		var rec inputRecord
		if err := rows.Scan(&rec.ID, &rec.ResultFilename); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

type lineRec struct {
	fields  []string // the 7 tab-separated fields
	ok      bool     // false for malformed lines
	page    int
	lineTyp string
	content string
}

func parseLineFile(content string) []lineRec {
	rawLines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	out := make([]lineRec, len(rawLines))
	for i, l := range rawLines {
		fields := strings.Split(l, "\t")
		if len(fields) != 7 {
			continue
		}
		page, err := strconv.Atoi(strings.TrimSpace(fields[1]))
		if err != nil {
			continue
		}
		out[i] = lineRec{
			fields:  fields,
			ok:      true,
			page:    page,
			lineTyp: strings.TrimSpace(fields[2]),
			content: strings.TrimSpace(fields[6]),
		}
	}
	return out
}

// backfillRecord patches the coordinate field of list-item lines in a
// record's canonical .txt line file, using the per-item bboxes
// parser_mineru.py's annotate_list_item_bboxes already computed and stored
// as list_item_bboxes on the consolidated JSON. Returns the number of lines
// patched.
func backfillRecord(rec inputRecord, dryRun bool) (int, error) {
	jsonPath := pathutil.ResolveDataHomePath(rec.ResultFilename)
	if jsonPath == "" {
		return 0, fmt.Errorf("empty result_filename")
	}
	jsonRaw, err := os.ReadFile(jsonPath)
	if err != nil {
		return 0, fmt.Errorf("read consolidated json: %w", err)
	}
	var doc mineruDoc
	if err := json.Unmarshal(jsonRaw, &doc); err != nil {
		return 0, fmt.Errorf("parse consolidated json: %w", err)
	}

	txtPath := rawLinePathFor(jsonPath)
	rawContent, err := os.ReadFile(txtPath)
	if err != nil {
		return 0, fmt.Errorf("read .txt: %w", err)
	}
	hadTrailingNewline := strings.HasSuffix(string(rawContent), "\n")
	txtLines := strings.Split(strings.TrimSuffix(string(rawContent), "\n"), "\n")
	parsed := parseLineFile(string(rawContent))

	// Group .txt line indices by page, preserving document order, so each
	// list can be searched for on its own page only.
	linesByPage := map[int][]int{}
	for i, l := range parsed {
		if l.ok {
			linesByPage[l.page] = append(linesByPage[l.page], i)
		}
	}

	// consumed tracks .txt indices already matched to a list, so a repeated
	// or ambiguous content string can't be matched twice.
	consumed := map[int]bool{}

	fixed := 0
	fuzzyMatched := 0
	for _, page := range doc.Pages {
		candidates := linesByPage[page.PageNumber]
		for _, item := range page.Items {
			if item.Type != "list" || len(item.ListItemBBoxes) != len(item.ListItems) || len(item.ListItems) == 0 {
				continue
			}
			matchAt := findContentRun(parsed, candidates, consumed, item.ListItems)
			if matchAt == nil {
				// The .txt text can drift from content_list.json through hand
				// correction after parse; fall back to a similarity match
				// before giving up (see findContentRunFuzzy).
				matchAt = findContentRunFuzzy(parsed, candidates, consumed, item.ListItems)
				if matchAt != nil {
					fuzzyMatched += len(matchAt)
					log.Printf("  page %d: list starting %q matched by similarity (text edited after parse)",
						page.PageNumber, truncate(item.ListItems[0], 30))
				}
			}
			if matchAt == nil {
				log.Printf("  page %d: no .txt run matches list starting %q — skipped",
					page.PageNumber, truncate(item.ListItems[0], 30))
				continue
			}
			for k, idx := range matchAt {
				consumed[idx] = true
				fixed++
				if !dryRun {
					parsed[idx].fields[5] = formatBBox(item.ListItemBBoxes[k])
					txtLines[idx] = strings.Join(parsed[idx].fields, "\t")
				}
			}
		}
	}

	if fuzzyMatched > 0 {
		log.Printf("  %d line(s) located by similarity fallback", fuzzyMatched)
	}
	if fixed == 0 || dryRun {
		return fixed, nil
	}

	out := strings.Join(txtLines, "\n")
	if hadTrailingNewline {
		out += "\n"
	}
	if err := os.WriteFile(txtPath, []byte(out), 0o644); err != nil {
		return fixed, fmt.Errorf("write .txt: %w", err)
	}
	return fixed, nil
}

// findContentRun finds a maximal run of consecutive (in document order,
// among unconsumed candidate indices) lines whose normalized content
// matches wantContent exactly, in order. Returns the matched .txt indices
// (same length as wantContent) or nil if no run matches.
func findContentRun(parsed []lineRec, candidates []int, consumed map[int]bool, wantContent []string) []int {
	n := len(wantContent)
	want := make([]string, n)
	for i, c := range wantContent {
		want[i] = normalizeText(c)
	}
	for start := 0; start+n <= len(candidates); start++ {
		ok := true
		for k := range n {
			idx := candidates[start+k]
			if consumed[idx] || normalizeText(parsed[idx].content) != want[k] {
				ok = false
				break
			}
		}
		if ok {
			out := make([]int, n)
			copy(out, candidates[start:start+n])
			return out
		}
	}
	return nil
}

// minRunSimilarity is the per-item floor for the fuzzy fallback. Post-parse
// human corrections are typically a character or two in a sentence of tens to
// hundreds (record 416 page 11: 2 chars in 247, similarity 0.992), while two
// genuinely different list items in the same document score far below this.
const minRunSimilarity = 0.9

// findContentRunFuzzy is the fallback for findContentRun. The .txt content is
// not, in fact, immutable after parse: it gets hand-corrected (record 416 page
// 11 had 震荡機 -> 振荡機 fixed by hand), and because findContentRun requires
// every item in the run to match exactly, one edited character silently skips
// the whole list -- leaving every line in it with a stale, wrong-coordinate-
// space bbox that then mis-paints highlights.
//
// This keeps findContentRun's structural guarantees (same page, contiguous
// unconsumed run, exact item count) and relaxes only the text comparison to a
// per-item similarity floor. It additionally requires every matched line to be
// a list-item, which findContentRun does not: exact text match is its own proof
// of identity, but a fuzzy match is not, and writing a list bbox over a
// paragraph that structure analysis merged would be a silent corruption.
// Among qualifying runs it returns the highest-scoring one, so a near-duplicate
// list elsewhere on the page cannot win over the real match.
func findContentRunFuzzy(parsed []lineRec, candidates []int, consumed map[int]bool, wantContent []string) []int {
	n := len(wantContent)
	if n == 0 {
		return nil
	}
	want := make([]string, n)
	for i, c := range wantContent {
		want[i] = normalizeText(c)
	}

	var best []int
	bestScore := 0.0
	for start := 0; start+n <= len(candidates); start++ {
		total := 0.0
		ok := true
		for k := range n {
			idx := candidates[start+k]
			if consumed[idx] || !isListItemType(parsed[idx].lineTyp) {
				ok = false
				break
			}
			sim := runeSimilarity(normalizeText(parsed[idx].content), want[k])
			if sim < minRunSimilarity {
				ok = false
				break
			}
			total += sim
		}
		if ok && total > bestScore {
			bestScore = total
			best = append(best[:0:0], candidates[start:start+n]...)
		}
	}
	return best
}

// isListItemType reports whether a .txt line type is one of the list-item
// variants the converter emits (list-item, list-item-num, list-item_m-sym,
// list-item-s-sym). Matching the bare "list-item" string would silently
// exclude the ~2.3k numbered and symbol-bulleted list lines in the corpus.
func isListItemType(t string) bool {
	return strings.HasPrefix(t, "list-item")
}

// runeSimilarity returns 1 - normalizedLevenshtein(a, b), over runes so CJK
// text is compared by character rather than by UTF-8 byte.
func runeSimilarity(a, b string) float64 {
	if a == b {
		return 1
	}
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}
	longest := max(len(ra), len(rb))
	// Cheap length prefilter: the edit distance is at least the length
	// difference, so an over-long candidate cannot clear the floor.
	if float64(longest-min(len(ra), len(rb)))/float64(longest) > 1-minRunSimilarity {
		return 0
	}

	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(min(cur[j-1]+1, prev[j]+1), prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return 1 - float64(prev[len(rb)])/float64(longest)
}

func rawLinePathFor(resultFilename string) string {
	dir := filepath.Dir(resultFilename)
	base := filepath.Base(resultFilename)
	ext := filepath.Ext(base)
	root := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, root+".txt")
}

var wsRe = regexp.MustCompile(`\s+`)

func normalizeText(s string) string {
	return wsRe.ReplaceAllString(s, "")
}

func formatBBox(b []float64) string {
	if len(b) != 4 {
		return ""
	}
	return fmt.Sprintf("[%s, %s, %s, %s]", fmtNum(b[0]), fmtNum(b[1]), fmtNum(b[2]), fmtNum(b[3]))
}

func fmtNum(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
