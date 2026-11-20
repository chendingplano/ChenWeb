package docsync

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/docsync/transfer"
	"github.com/chendingplano/deepdoc/server/api/pathutil"
	"github.com/lib/pq"
)

type Source struct {
	DB           *sql.DB
	ArtifactRoot string
}

func (s Source) Callbacks() transfer.Callbacks {
	return transfer.Callbacks{List: s.list, Pack: s.pack}
}

const inputFields = `'type', i.type, 'title', i.title, 'doc_no', i.doc_no,
	'source', i.source, 'authors', i.authors, 'publish_date', i.publish_date,
	'public_info', i.public_info, 'notes', i.notes, 'ks_desc', i.ks_desc,
	'staging_filename', i.staging_filename, 'parser_name', i.parser_name`

type documentData struct {
	Metadata json.RawMessage   `json:"metadata"`
	Owner    string            `json:"source_owner"`
	Statuses []json.RawMessage `json:"statuses"`
}

type sourceInput struct {
	id        int64
	modified  time.Time
	hash      string
	data      documentData
	original  string
	result    string
	status    map[string]json.RawMessage
	duplicate bool
}

type snapshot struct {
	item  workItem
	data  any
	files []sourceFile
	rows  int
}

type sourceFile struct {
	fileEntry
	path string
}

type processorStatus struct {
	Operation string `json:"operation"`
	Proc      string `json:"proc_status"`
	Legacy    string `json:"proc-status"`
	Status    string `json:"status"`
	Start     string `json:"start_time"`
}

func statusValue(raw json.RawMessage) string {
	var status processorStatus
	_ = json.Unmarshal(raw, &status)
	for _, val := range []string{status.Proc, status.Legacy, status.Status} {
		if val = strings.ToLower(strings.TrimSpace(val)); val != "" {
			return val
		}
	}
	return ""
}

func statusKeys(op string) []string {
	op = docprocessing.SyncOperationName(op)
	if op == "extract_entity_relation" {
		return []string{"extract_entity", "extract_relation"}
	}
	for _, processor := range []string{
		"chunking", "static_analyzer", "extract_doc_metadata", "generate_scene_blocks",
		"extract_semantic_projections", "extract_structured_knowledge",
	} {
		if slices.Contains(docprocessing.SyncProcessorAliases(processor), op) {
			return []string{processor}
		}
	}
	switch op {
	case "parsed", "parse", "parsing":
		return []string{"parsed"}
	case "convert", "converted", "converting":
		return []string{"converted"}
	case "", "consumed", "stop_requested", "doc_processor":
		return nil
	}
	return []string{op}
}

func readSourceInput(ctx context.Context, tx *sql.Tx, id int64) (sourceInput, error) {
	input := sourceInput{id: id, status: make(map[string]json.RawMessage)}
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT modify_time, COALESCE(md5,''), COALESCE(user_id::text,''),
		COALESCE(file_name,''), COALESCE(result_filename,''), status, jsonb_build_object(`+inputFields+`)
		FROM kb.inputs i WHERE i.id=$1`, id).Scan(
		&input.modified, &input.hash, &input.data.Owner, &input.original, &input.result, &raw, &input.data.Metadata)
	if err != nil {
		return input, err
	}
	input.hash = normalizeMD5(input.hash)
	var statuses []json.RawMessage
	if err = json.Unmarshal(raw, &statuses); err != nil {
		return input, err
	}

	// The rollup determines which latest operations exist. Preserve their original
	// array entries, including legacy names and any fields consumed by the UI.
	rows, err := tx.QueryContext(ctx, `SELECT processor FROM kb.input_proc_status WHERE record_id=$1`, id)
	if err != nil {
		return input, err
	}
	defer rows.Close()
	tracked := map[string]bool{pipelineItem: true, "parsed": true, "converted": true}
	for rows.Next() {
		var op string
		if err := rows.Scan(&op); err != nil {
			return input, err
		}
		for _, key := range statusKeys(op) {
			tracked[key] = true
		}
	}
	if err := rows.Err(); err != nil {
		return input, err
	}
	for _, entry := range statuses {
		var st processorStatus
		if err := json.Unmarshal(entry, &st); err != nil {
			return input, err
		}
		for _, key := range statusKeys(st.Operation) {
			if tracked[key] {
				normalized, err := compactStatus(entry)
				if err != nil {
					return input, err
				}
				input.status[key] = normalized
			}
		}
	}
	input.duplicate = statusValue(input.status["parsed"]) == "duplicated"
	for _, name := range []string{"parsed", "converted", "static_analyzer"} {
		if statusValue(input.status[name]) == "success" {
			input.data.Statuses = append(input.data.Statuses, input.status[name])
		}
	}
	return input, nil
}

func sourceWinner(ctx context.Context, tx *sql.Tx, hash string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM kb.inputs WHERE md5 IS NOT NULL AND lower(btrim(md5))=$1
		AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(status) WITH ORDINALITY AS s(entry,n)
			WHERE n=(SELECT max(t.n) FROM jsonb_array_elements(status) WITH ORDINALITY AS t(entry,n)
				WHERE kb.canonical_op(t.entry->>'operation') IN ('parsed','parsing','parse'))
			AND lower(coalesce(nullif(entry->>'proc_status',''),nullif(entry->>'proc-status',''),entry->>'status',''))='duplicated')
		ORDER BY (parse_state='parsed_success') DESC,id LIMIT 1`, hash).Scan(&id)
	return id, err
}

func (s Source) list(ctx context.Context, raw json.RawMessage, out io.Writer) error {
	var req listRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	if req.Until.IsZero() {
		if err := s.DB.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&req.Until); err != nil {
			return err
		}
		return enc.Encode(listing{Until: req.Until})
	}
	if req.Vocabulary {
		tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if !req.All && !req.After.Time.IsZero() {
			changed, err := VocabularyChanged(ctx, tx, req.After.Time, req.Until)
			if err != nil {
				return err
			}
			if !changed {
				return nil
			}
		}
		item := workItem{Kind: VocabularyProcessor, SeenAt: req.Until}
		snap, err := s.snapshot(ctx, tx, sourceInput{}, item)
		if err != nil {
			item.Issue = err.Error()
		} else {
			item = snap.item
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		return enc.Encode(listing{Items: []workItem{item}})
	}
	query := `SELECT id, modify_time FROM kb.inputs WHERE (modify_time,id)>($1,$2) AND modify_time <= $3 ORDER BY modify_time,id LIMIT $4`
	args := []any{req.After.Time, req.After.ID, req.Until, pageSize}
	if len(req.IDs) > 0 {
		if len(req.IDs) > pageSize {
			return errors.New("too many requested source IDs")
		}
		query = `SELECT id, modify_time FROM kb.inputs WHERE id=ANY($1) AND modify_time <= $2 ORDER BY id`
		args = []any{pq.Array(req.IDs), req.Until}
	} else if req.All {
		query = `SELECT id, modify_time FROM kb.inputs WHERE id>$1 AND modify_time <= $2 ORDER BY id LIMIT $3`
		args = []any{req.After.ID, req.Until, pageSize}
	}
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	var page []cursor
	for rows.Next() {
		var current cursor
		if err := rows.Scan(&current.ID, &current.Time); err != nil {
			rows.Close()
			return err
		}
		page = append(page, current)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, current := range page {
		line, err := s.listInput(ctx, current, req.Until)
		if err != nil {
			return err
		}
		if err := enc.Encode(line); err != nil {
			return err
		}
	}
	return nil
}

func (s Source) listInput(ctx context.Context, current cursor, upper time.Time) (listing, error) {
	line := listing{Cursor: &current}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return line, err
	}
	defer tx.Rollback()
	input, err := readSourceInput(ctx, tx, current.ID)
	if errors.Is(err, sql.ErrNoRows) {
		line.Issue = "source document was removed"
		return line, nil
	}
	if err != nil || input.modified.After(upper) {
		return line, err
	}
	if input.hash == "" {
		line.Issue = "source document has missing or invalid MD5"
		return line, nil
	}
	line.Hash = input.hash
	if input.duplicate {
		line.Issue = "source parse marked this document as duplicated"
		return line, nil
	}
	winner, err := sourceWinner(ctx, tx, input.hash)
	if err != nil {
		return line, err
	}
	if winner != current.ID {
		line.Issue = fmt.Sprintf("source document shares its hash with selected input %d", winner)
		return line, nil
	}
	status := make(map[string]string)
	names := successfulProcessors(input.status)
	// Older builds persist facet observations inline without their own
	// status. Carry those rows while leaving the original status array intact.
	for _, name := range []string{"facet_tier1", "facet_tier2", "classify_document"} {
		if input.status[name] == nil {
			names = append(names, name)
		}
	}
	for _, kind := range append([]string{documentItem}, names...) {
		item := workItem{SourceID: input.id, Kind: kind, Hash: input.hash, SeenAt: upper, Status: input.status[kind]}
		snap, err := s.snapshot(ctx, tx, input, item)
		legacyFacet := kind != documentItem && item.Status == nil
		switch {
		case err != nil:
			item.Issue = err.Error()
			status[kind] = ""
		case legacyFacet && snap.rows == 0:
			continue
		default:
			item = snap.item
			status[kind] = item.Digest
		}
		line.Items = append(line.Items, item)
	}
	for i := range line.Items {
		item := &line.Items[i]
		if item.Kind == documentItem {
			continue
		}
		item.Requires = map[string]string{documentItem: status[documentItem]}
		for _, dep := range ProcessorDependencies(item.Kind) {
			if digest, listed := status[dep]; listed {
				item.Requires[dep] = digest
			} else {
				// If production did not report a successful dependency, linking
				// still checks that any referenced rows are present locally.
				item.Requires[dep] = "*"
			}
		}
	}
	if ps := statusValue(input.status[pipelineItem]); slices.Contains([]string{"success", "fail", "failed", "stopped"}, ps) {
		item := workItem{SourceID: input.id, Hash: input.hash, Kind: pipelineItem, SeenAt: upper,
			Status: input.status[pipelineItem], Requires: status}
		for _, processor := range successfulProcessors(input.status) {
			// Unknown or currently unavailable processors block the summary.
			if _, ok := status[processor]; !ok {
				status[processor] = ""
			}
		}
		item.Digest, err = digest(struct {
			Status   json.RawMessage
			Required map[string]string
		}{item.Status, status})
		if err != nil {
			return line, err
		}
		line.Items = append(line.Items, item)
	}
	return line, tx.Commit()
}

func successfulProcessors(status map[string]json.RawMessage) []string {
	var names []string
	for name, entry := range status {
		switch name {
		case "parsed", "converted", "static_analyzer", pipelineItem, "extract_metrics", "extract_metrics_simple":
			continue
		}
		if statusValue(entry) == "success" {
			names = append(names, name)
		}
	}
	// The metric implementations share one output slot. Use the newest
	// writer's status so a historical success cannot claim its newer rows.
	writer := "extract_metrics"
	other := "extract_metrics_simple"
	var first, second processorStatus
	_ = json.Unmarshal(status[writer], &first)
	_ = json.Unmarshal(status[other], &second)
	if status[writer] == nil || second.Start > first.Start {
		writer = other
	}
	if statusValue(status[writer]) == "success" {
		names = append(names, writer)
	}
	slices.Sort(names)
	return names
}

func (s Source) snapshot(ctx context.Context, tx *sql.Tx, input sourceInput, item workItem) (snapshot, error) {
	result := snapshot{item: item}
	var err error
	var contentDigest string
	switch item.Kind {
	case documentItem:
		result.data = input.data
		result.rows = 1
		contentDigest, err = digest(input.data)
	case pipelineItem:
		result.data = struct{}{}
		contentDigest = item.Digest
	default:
		if !HasContract(item.Kind) {
			return result, fmt.Errorf("processor %q has no export contract", item.Kind)
		}
		var bundle Bundle
		bundle, err = ExportBundle(ctx, tx, item.Kind, input.id)
		if err != nil {
			return result, err
		}
		result.data = bundle
		for _, rows := range bundle.Rows {
			result.rows += len(rows)
		}
		contentDigest, err = bundle.Digest()
	}
	if err != nil {
		return result, err
	}
	if item.Kind == pipelineItem {
		return result, nil
	}
	if item.Kind != VocabularyProcessor {
		result.files, err = s.files(input, item.Kind)
	}
	if err != nil {
		return result, err
	}
	files := make([]fileEntry, 0, len(result.files))
	result.item.EstimateBytes = int64(result.rows) * 1024
	for _, file := range result.files {
		files = append(files, file.fileEntry)
		result.item.EstimateBytes += file.Size
	}
	result.item.Digest, err = digest(struct {
		Content string
		Files   []fileEntry
	}{contentDigest, files})
	return result, err
}

func describeFile(name, path, md5Hex string) (sourceFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return sourceFile{}, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return sourceFile{}, err
	}
	if !stat.Mode().IsRegular() {
		return sourceFile{}, errors.New("artifact is not a regular file")
	}
	strong, hash := sha256.New(), md5.New()
	size, err := io.Copy(io.MultiWriter(strong, hash), f)
	if err != nil {
		return sourceFile{}, err
	}
	if md5Hex != "" && md5Hex != fmt.Sprintf("%x", hash.Sum(nil)) {
		return sourceFile{}, errors.New("source original file does not match recorded MD5")
	}
	return sourceFile{fileEntry: fileEntry{Name: name, Size: size, SHA256: fmt.Sprintf("%x", strong.Sum(nil))}, path: path}, nil
}

func recordDirectory(root string, id int64) string {
	return filepath.Join(root, fmt.Sprint(id/1000), fmt.Sprint(id))
}

func (s Source) files(input sourceInput, kind string) ([]sourceFile, error) {
	var files []sourceFile
	add := func(name, path, hash string) error {
		f, err := describeFile(name, path, hash)
		if err != nil {
			return fmt.Errorf("read source %s: %w", name, err)
		}
		files = append(files, f)
		return nil
	}
	resultPath := pathutil.ResolveDataHomePath(input.result)
	if kind == documentItem {
		if err := add("original", pathutil.ResolveDataHomePath(input.original), input.hash); err != nil {
			return nil, err
		}
		if statusValue(input.status["parsed"]) == "success" || statusValue(input.status["converted"]) == "success" {
			if resultPath == "" {
				return nil, errors.New("parsed document has no result file")
			}
			if err := add("result"+filepath.Ext(resultPath), resultPath, ""); err != nil {
				return nil, err
			}
		}
	}
	dir := recordDirectory(s.ArtifactRoot, input.id)
	var metadata struct {
		Staging string `json:"staging_filename"`
	}
	if err := json.Unmarshal(input.data.Metadata, &metadata); err != nil {
		return nil, err
	}
	parseComplete := statusValue(input.status["parsed"]) == "success" || statusValue(input.status["converted"]) == "success"
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if entry.IsDir() {
			if name == "." || (kind == documentItem && parseComplete && (name == "images" || strings.HasPrefix(name, "images/"))) {
				return nil
			}
			return filepath.SkipDir
		}
		if !entry.Type().IsRegular() || path == resultPath || !ownsArtifact(kind, name, input.result, metadata.Staging, input.status) {
			return nil
		}
		return add("artifacts/"+name, path, "")
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return files, nil
}

func filenameStem(name string) string {
	return strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
}

func ownsArtifact(kind, name, result, staging string, status map[string]json.RawMessage) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch kind {
	case documentItem:
		if ext == ".corrected" {
			return statusValue(status["static_analyzer"]) == "success"
		}
		parseComplete := statusValue(status["parsed"]) == "success" || statusValue(status["converted"]) == "success"
		if !parseComplete {
			return false
		}
		root := filenameStem(staging)
		return strings.HasPrefix(name, "images/") || name == root+"_content_list.json" ||
			name == root+".pdf-table-geometry.json" ||
			(slices.Contains([]string{".json", ".md", ".html", ".txt", ".jsonl", ".zip"}, ext) &&
				strings.HasPrefix(name, filenameStem(result)))
	case "chunking":
		return ext == ".chunks"
	case "generate_topics":
		return ext == ".topics"
	case "generate_summaries":
		return strings.HasPrefix(name, "summary_") && ext == ".txt"
	case "structure_analyzer":
		return name == "structure_labels.jsonl" || name == "structure_summary.json"
	case "extract_metrics", "extract_metrics_simple":
		return ext == ".metrics"
	case "extract_provisions":
		return ext == ".provisions"
	case "extract_products":
		return ext == ".products"
	case "extract_entity":
		return ext == ".entities"
	case "extract_relation":
		return ext == ".relations"
	case "extract_inventory_items":
		return ext == ".inventory_items"
	case "extract_semantic_projections":
		return ext == ".semantic_projections"
	case "extract_structured_knowledge":
		return ext == ".knowledges"
	case "generate_scene_blocks":
		return ext == ".scene_blocks"
	}
	return false
}
