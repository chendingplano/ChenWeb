package docsync

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/pathutil"
)

var errProcessing = errors.New("document is being processed locally")

type localDocument struct {
	ID       int64
	Origin   string
	Staging  string
	Parser   string
	Original string
	Result   string
	SourceID int64
}

type localFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type applyResult struct {
	InputID   int64
	Converted bool
	Inserted  bool
	Skipped   bool
	Removed   []string
	Conflicts []string
	Rows      int
	Files     int
}

func (r *Runner) resolveInput(ctx context.Context, q queryer, work workItem) (localDocument, error) {
	var d localDocument
	err := q.QueryRowContext(ctx, `SELECT i.id, i.doc_origin, COALESCE(i.staging_filename,''),
		COALESCE(i.parser_name,''), COALESCE(i.file_name,''), COALESCE(i.result_filename,''), m.source_id
		FROM kb.doc_sync_inputs m JOIN kb.inputs i ON i.id=m.input_id
		WHERE m.site=$1 AND m.source_id=$2 AND m.md5=$3`, r.Site, work.SourceID, work.Hash).Scan(
		&d.ID, &d.Origin, &d.Staging, &d.Parser, &d.Original, &d.Result, &d.SourceID)
	if !errors.Is(err, sql.ErrNoRows) {
		return d, err
	}
	err = q.QueryRowContext(ctx, `SELECT count(*) FROM kb.inputs WHERE md5 IS NOT NULL AND lower(btrim(md5))=$1`, work.Hash).Scan(&d.ID)
	if err != nil {
		return localDocument{}, err
	}
	switch d.ID {
	case 0:
		return localDocument{}, nil
	case 1:
	default:
		return localDocument{}, errors.New("more than one local document has the source MD5")
	}
	err = q.QueryRowContext(ctx, `SELECT i.id, i.doc_origin, COALESCE(i.staging_filename,''),
		COALESCE(i.parser_name,''), COALESCE(i.file_name,''), COALESCE(i.result_filename,''),
		COALESCE((SELECT source_id FROM kb.doc_sync_inputs WHERE site=$2 AND input_id=i.id),0)
		FROM kb.inputs i WHERE md5 IS NOT NULL AND lower(btrim(md5))=$1
		ORDER BY id LIMIT 1`, work.Hash, r.Site).Scan(
		&d.ID, &d.Origin, &d.Staging, &d.Parser, &d.Original, &d.Result, &d.SourceID)
	return d, err
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *Runner) markAndLease(ctx context.Context, id int64, cancel context.CancelFunc) (func(), error) {
	if _, err := r.DB.ExecContext(ctx, `INSERT INTO kb.doc_sync_marks (input_id) VALUES ($1) ON CONFLICT DO NOTHING`, id); err != nil {
		return nil, err
	}
	unmark := func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := r.DB.ExecContext(cleanup, `DELETE FROM kb.doc_sync_marks WHERE input_id=$1`, id); err != nil {
			r.Logger.Error("clear document sync mark", "input_id", id, "error", err)
		}
	}
	release, acquired, err := docprocessing.AcquireDocumentLease(ctx, r.DB, id, cancel)
	if err != nil {
		unmark()
		return nil, err
	}
	cleanup := func() {
		unmark()
		if acquired {
			release()
		}
	}
	var active bool
	err = r.DB.QueryRowContext(ctx, `SELECT
		EXISTS (SELECT 1 FROM kb.doc_process_runs WHERE record_id=$1 AND status='running') OR
		EXISTS (SELECT 1 FROM kb.inputs i CROSS JOIN LATERAL jsonb_each(
			(SELECT coalesce(jsonb_object_agg(kb.canonical_op(s.entry->>'operation'), s.entry ORDER BY s.n), '{}'::jsonb)
			 FROM jsonb_array_elements(i.status) WITH ORDINALITY s(entry,n)
			 WHERE trim(coalesce(s.entry->>'operation','')) <> '')
		) p WHERE i.id=$1 AND lower(coalesce(p.value->>'proc_status', p.value->>'proc-status', p.value->>'status',''))
			IN ('active','running','in_progress','parsing','converting'))`, id).Scan(&active)
	if err != nil {
		cleanup()
		return nil, err
	}
	if active || !acquired {
		err = (docprocessing.StopRequestSQLStore{DB: r.DB}).SetStopRequested(ctx, id)
		cleanup()
		return nil, errors.Join(errProcessing, err)
	}
	return cleanup, nil
}

func (r *Runner) apply(ctx context.Context, owner, staging string, item manifestItem) (result applyResult, retErr error) {
	work := item.Work
	var local localDocument
	var err error
	if work.Kind != VocabularyProcessor {
		local, err = r.resolveInput(ctx, r.DB, work)
		if err != nil {
			return result, err
		}
		if local.ID != 0 {
			result.InputID = local.ID
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			defer cancel()
			release, err := r.markAndLease(ctx, local.ID, cancel)
			if err != nil {
				return result, err
			}
			defer release()
		}
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if local.ID != 0 {
		var lockID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM kb.inputs WHERE id=$1 FOR UPDATE`, local.ID).Scan(&lockID); err != nil {
			return result, err
		}
	}
	current, err := r.version(ctx, tx, work.SourceID, work.Kind)
	if err != nil {
		return result, err
	}
	if work.Kind != VocabularyProcessor && local.ID == 0 && work.Kind == documentItem && current.Digest != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM kb.doc_sync_applied WHERE site=$1 AND source_id=$2`, r.Site, work.SourceID); err != nil {
			return result, err
		}
		current = appliedVersion{}
	}
	if current.same(work) || current.Time.After(work.SeenAt) {
		result.Skipped = true
		return result, nil
	}
	for kind, hash := range work.Requires {
		if hash == "*" {
			continue
		}
		applied, err := r.version(ctx, tx, work.SourceID, kind)
		if err != nil {
			return result, err
		}
		if hash == "" || applied.Digest != hash {
			return result, fmt.Errorf("waiting for %s dependency", kind)
		}
	}

	raw, err := readItemRows(staging, item)
	if err != nil {
		return result, err
	}
	oldFiles := make(map[string]localFile)
	for k, v := range current.Files {
		oldFiles[k] = v
	}
	j := fileJournal{Site: r.Site, SourceID: work.SourceID, Kind: work.Kind, Digest: work.Digest,
		SeenAt: work.SeenAt.Format(time.RFC3339Nano)}
	if work.Kind == documentItem {
		var document documentData
		if err := json.Unmarshal(raw, &document); err != nil {
			return result, err
		}
		var provider struct {
			Staging string `json:"staging_filename"`
			Parser  string `json:"parser_name"`
		}
		if err := json.Unmarshal(document.Metadata, &provider); err != nil {
			return result, err
		}
		result.Inserted = local.ID == 0
		result.Converted = local.Origin == "local"
		if result.Inserted {
			local.Staging = provider.Staging
			// No ordinary upload workflow or JetStream events are generated.
			if err := tx.QueryRowContext(ctx, `INSERT INTO kb.inputs (type,user_id,ks_store_id,doc_origin,processing_mode)
				VALUES ($1,$2,$3,'remote','upload_only') RETURNING id`, "document", owner, r.StoreID).Scan(&local.ID); err != nil {
				return result, err
			}
			result.InputID = local.ID
		} else {
			if _, err := describeFile("original", pathutil.ResolveDataHomePath(local.Original), work.Hash); err != nil {
				return result, fmt.Errorf("verify existing original: %w", err)
			}
		}
		invalidate := result.Converted || (!result.Inserted && local.SourceID != work.SourceID) ||
			(current.Digest != "" && documentFilesChanged(current.Files, item.Files))
		if invalidate {
			result.Removed, err = r.clearDependent(ctx, tx, work, local.ID, true, oldFiles)
			if err != nil {
				return result, err
			}
		}
		if local.Result != "" && (result.Converted || invalidate) {
			if err := r.trackOldResult(ctx, tx, local, oldFiles); err != nil {
				return result, err
			}
		}
		if err := r.applyDocument(ctx, tx, owner, &local, item, document, provider.Staging, provider.Parser, invalidate); err != nil {
			return result, err
		}
	} else if work.Kind != VocabularyProcessor {
		if local.ID == 0 || local.SourceID != work.SourceID {
			return result, errors.New("waiting for document item")
		}
		if work.Kind != pipelineItem && current.Digest != work.Digest {
			if sibling := metricSibling(work.Kind); sibling != "" {
				if err := writeStatus(ctx, tx, local.ID, sibling, nil); err != nil {
					return result, err
				}
				if _, err := tx.ExecContext(ctx, `DELETE FROM kb.doc_sync_applied WHERE site=$1 AND source_id=$2 AND processor=$3`,
					r.Site, work.SourceID, sibling); err != nil {
					return result, err
				}
			}
			result.Removed, err = r.clearDependent(ctx, tx, work, local.ID, false, oldFiles)
			if err != nil {
				return result, err
			}
		}
	}

	if work.Kind != documentItem && work.Kind != pipelineItem && current.Digest != work.Digest {
		var bundle Bundle
		if err := json.Unmarshal(raw, &bundle); err != nil {
			return result, err
		}
		applied, err := ApplyBundle(ctx, tx, r.Site, work.Kind, work.SourceID, local.ID, bundle)
		if err != nil {
			return result, err
		}
		result.Rows = applied.Rows
		result.Conflicts = applied.Conflicts
		if err := r.queueIndex(ctx, tx, local.ID, work.Kind); err != nil {
			return result, err
		}
	}

	if work.Kind != documentItem && work.Kind != VocabularyProcessor {
		if err := writeStatus(ctx, tx, local.ID, work.Kind, localStatus(work)); err != nil {
			return result, err
		}
	}
	files := current.Files
	if work.Kind == documentItem || current.Digest != work.Digest {
		files, err = r.planFiles(staging, item, local, oldFiles, &j, result.Converted, result.Removed)
		if err != nil {
			return result, err
		}
	}
	result.Files = len(j.Changes)
	fileJSON, err := json.Marshal(files)
	if err != nil {
		return result, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO kb.doc_sync_applied
		(site,source_id,processor,digest,source_time,source_status,files)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT(site,source_id,processor) DO UPDATE SET
		digest=excluded.digest, source_time=excluded.source_time, source_status=excluded.source_status, files=excluded.files`,
		r.Site, work.SourceID, work.Kind, work.Digest, work.SeenAt, nullableJSON(work.Status), string(fileJSON))
	if err != nil {
		return result, err
	}
	if len(j.Changes) == 0 {
		return result, tx.Commit()
	}
	if err := j.prepare(r.Inbox); err != nil {
		return result, err
	}
	defer func() {
		_ = tx.Rollback()
		check, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		committed, err := journalCommitted(check, r.DB, j)
		if err == nil {
			err = j.finish(committed)
		}
		retErr = errors.Join(retErr, err)
	}()
	if err := j.place(ctx); err != nil {
		return result, err
	}
	return result, tx.Commit()
}

type appliedVersion struct {
	Digest string
	Time   time.Time
	Status json.RawMessage
	Files  map[string]localFile
}

func (v appliedVersion) same(item workItem) bool {
	return v.Digest == item.Digest && item.Digest != "" && bytes.Equal(v.Status, item.Status)
}

func (r *Runner) version(ctx context.Context, q queryer, id int64, kind string) (appliedVersion, error) {
	var v appliedVersion
	var files []byte
	var raw []byte
	err := q.QueryRowContext(ctx, `SELECT digest,source_time,source_status,files FROM kb.doc_sync_applied
		WHERE site=$1 AND source_id=$2 AND processor=$3`, r.Site, id, kind).Scan(&v.Digest, &v.Time, &raw, &files)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	if len(raw) > 0 {
		// jsonb may change whitespace and key ordering relative to a marshaled manifest.
		v.Status, err = compactStatus(raw)
		if err != nil {
			return v, err
		}
	}
	return v, json.Unmarshal(files, &v.Files)
}

func compactStatus(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var data any
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}
	return json.Marshal(data)
}

func nullableJSON(v json.RawMessage) any {
	if len(v) == 0 {
		return nil
	}
	return string(v)
}

func (r *Runner) applyDocument(ctx context.Context, tx *sql.Tx, owner string, d *localDocument,
	item manifestItem, document documentData, sourceStaging, parser string, resetStatus bool) error {
	if d.Staging == "" {
		return errors.New("input staging filename is missing")
	}
	d.Parser = parser
	if filepath.Base(d.Staging) != d.Staging || filepath.Base(sourceStaging) != sourceStaging ||
		strings.ContainsAny(d.Staging+sourceStaging+parser, "\\/:\x00") {
		return errors.New("invalid portable filename")
	}
	dir := recordDirectory(r.ArtifactRoot, d.ID)
	if d.Original == "" {
		original, err := localRelativePath(filepath.Join(dir, d.Staging))
		if err != nil {
			return err
		}
		d.Original = original
	}
	d.Result = ""
	for _, file := range item.Files {
		if strings.HasPrefix(file.Name, "result.") {
			name := docprocessing.SyncArtifactBaseName(d.Staging, parser) + filepath.Ext(file.Name)
			result, err := localRelativePath(filepath.Join(dir, name))
			if err != nil {
				return err
			}
			d.Result = result
		}
	}
	statuses := []json.RawMessage{}
	if !resetStatus {
		var raw []byte
		if err := tx.QueryRowContext(ctx, `SELECT status FROM kb.inputs WHERE id=$1`, d.ID).Scan(&raw); err != nil {
			return err
		}
		var previous []json.RawMessage
		if err := json.Unmarshal(raw, &previous); err != nil {
			return err
		}
		for _, entry := range previous {
			var st processorStatus
			if err := json.Unmarshal(entry, &st); err != nil {
				return err
			}
			keys := statusKeys(st.Operation)
			if len(keys) == 1 && slices.Contains([]string{"parsed", "converted", "static_analyzer"}, keys[0]) {
				continue
			}
			statuses = append(statuses, entry)
		}
	}
	statuses = append(statuses, document.Statuses...)
	statusJSON, err := json.Marshal(statuses)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE kb.inputs i SET
		type=s.type, title=s.title, doc_no=s.doc_no, source=s.source, authors=s.authors,
		publish_date=s.publish_date, public_info=s.public_info, notes=s.notes, ks_desc=s.ks_desc,
		private_info=NULL, doc_metadata=CASE WHEN $11 THEN NULL ELSE i.doc_metadata END, error_msg=NULL, backup_filename=NULL, owner=NULL,
		user_id=$3, ks_store_id=$4, staging_filename=$5, file_name=$6, result_filename=NULLIF($7,''),
		parser_name=NULLIF($8,''), md5=$9, doc_origin='remote', processing_mode='upload_only', requested_pipeline=NULL,
		status=$10::jsonb, modify_time=now()
		FROM jsonb_populate_record(NULL::kb.inputs,$2::jsonb) s
		WHERE i.id=$1`, d.ID, string(document.Metadata), owner, r.StoreID, d.Staging, d.Original, d.Result, parser, item.Work.Hash, string(statusJSON), resetStatus)
	if err != nil {
		return err
	}
	if d.SourceID != 0 && d.SourceID != item.Work.SourceID {
		if _, err := tx.ExecContext(ctx, `DELETE FROM kb.doc_sync_applied WHERE site=$1 AND source_id=$2`, r.Site, d.SourceID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM kb.doc_sync_inputs WHERE site=$1 AND input_id=$2`, r.Site, d.ID); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO kb.doc_sync_inputs(site,source_id,input_id,source_owner,md5)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT(site,source_id) DO UPDATE SET
		input_id=excluded.input_id, source_owner=excluded.source_owner, md5=excluded.md5`,
		r.Site, item.Work.SourceID, d.ID, document.Owner, item.Work.Hash)
	return err
}

func documentFilesChanged(old map[string]localFile, next []fileEntry) bool {
	if len(old) != len(next) {
		return true
	}
	for _, file := range next {
		if old[file.Name].SHA256 != file.SHA256 {
			return true
		}
	}
	return false
}

func (r *Runner) trackOldResult(ctx context.Context, tx *sql.Tx, doc localDocument, files map[string]localFile) error {
	path := pathutil.ResolveDataHomePath(doc.Result)
	if path == pathutil.ResolveDataHomePath(doc.Original) {
		return nil
	}
	rel, err := localRelativePath(path)
	if err != nil {
		return nil
	}
	var shared bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM kb.inputs WHERE id<>$1 AND
		(result_filename IN ($2,$3) OR file_name IN ($2,$3) OR backup_filename IN ($2,$3)))`,
		doc.ID, rel, path).Scan(&shared); err != nil {
		return err
	}
	if !shared {
		files["previous_result"] = localFile{Path: path}
	}
	return nil
}

func metricSibling(kind string) string {
	switch kind {
	case "extract_metrics":
		return "extract_metrics_simple"
	case "extract_metrics_simple":
		return "extract_metrics"
	}
	return ""
}

func localStatus(item workItem) json.RawMessage {
	var st processorStatus
	if json.Unmarshal(item.Status, &st) != nil || docprocessing.SyncOperationName(st.Operation) != "extract_entity_relation" {
		return item.Status
	}
	// The source's legacy entry describes both processors. Keep each
	// processor visible only once its own output has committed.
	var fields map[string]json.RawMessage
	if json.Unmarshal(item.Status, &fields) != nil {
		return item.Status
	}
	fields["operation"], _ = json.Marshal(item.Kind)
	status, _ := json.Marshal(fields)
	return status
}

func writeStatus(ctx context.Context, tx *sql.Tx, inputID int64, kind string, entry json.RawMessage) error {
	aliases := docprocessing.SyncProcessorAliases(kind)
	// The legacy combined entry also describes the sibling split processor.
	aliases = slices.DeleteFunc(aliases, func(alias string) bool { return alias == "extract_entity_relation" })
	aliasesJSON, err := json.Marshal(aliases)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE kb.inputs SET status=COALESCE((
		SELECT jsonb_agg(s.entry ORDER BY s.n) FROM jsonb_array_elements(status) WITH ORDINALITY s(entry,n)
		WHERE NOT ($2::jsonb ? kb.canonical_op(s.entry->>'operation'))
			AND ($3::jsonb IS NULL OR s.entry <> $3::jsonb)
		), '[]'::jsonb) || CASE WHEN $3::jsonb IS NULL THEN '[]'::jsonb ELSE jsonb_build_array($3::jsonb) END,
		modify_time=now() WHERE id=$1`, inputID, string(aliasesJSON), nullableJSON(entry))
	return err
}

func (r *Runner) clearDependent(ctx context.Context, tx *sql.Tx, work workItem, localID int64,
	all bool, files map[string]localFile) ([]string, error) {
	contracts := AllContracts()
	removed := map[string]bool{work.Kind: true}
	for _, kind := range contracts {
		if kind == VocabularyProcessor || kind == work.Kind {
			continue
		}
		for _, dep := range ProcessorDependencies(kind) {
			if removed[dep] {
				removed[kind] = true
			}
		}
		if all {
			removed[kind] = true
		}
	}
	delete(removed, work.Kind)
	var kinds []string
	for i := len(contracts) - 1; i >= 0; i-- {
		kind := contracts[i]
		if !removed[kind] {
			continue
		}
		if err := ClearProcessor(ctx, tx, r.Site, kind, localID); err != nil {
			return nil, fmt.Errorf("clear dependent %s: %w", kind, err)
		}
		kinds = append(kinds, kind)
		if kind == "extract_doc_metadata" {
			if _, err := tx.ExecContext(ctx, `UPDATE kb.inputs SET doc_metadata=NULL WHERE id=$1`, localID); err != nil {
				return nil, err
			}
		}
		if err := r.queueIndex(ctx, tx, localID, kind); err != nil {
			return nil, err
		}
		if err := writeStatus(ctx, tx, localID, kind, nil); err != nil {
			return nil, err
		}
	}
	kinds = append(kinds, pipelineItem)
	if err := writeStatus(ctx, tx, localID, pipelineItem, nil); err != nil {
		return nil, err
	}
	for _, kind := range kinds {
		rows, err := tx.QueryContext(ctx, `DELETE FROM kb.doc_sync_applied a USING kb.doc_sync_inputs i
			WHERE a.site=i.site AND a.source_id=i.source_id AND i.site=$1 AND i.input_id=$2 AND a.processor=$3
			RETURNING a.files`, r.Site, localID, kind)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var raw []byte
			var old map[string]localFile
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				return nil, err
			}
			if err := json.Unmarshal(raw, &old); err != nil {
				rows.Close()
				return nil, err
			}
			for key, file := range old {
				files[kind+"/"+key] = file
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return kinds, nil
}

func (r *Runner) planFiles(staging string, item manifestItem, d localDocument,
	old map[string]localFile, journal *fileJournal, converting bool, removed []string) (map[string]localFile, error) {
	files := make(map[string]localFile)
	if d.ID == 0 {
		return files, nil
	}
	dir := recordDirectory(r.ArtifactRoot, d.ID)
	if converting || len(removed) > 0 {
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			owned := converting
			for _, kind := range removed {
				owned = owned || ownsArtifact(kind, entry.Name(), "", "", nil)
			}
			if entry.Type().IsRegular() && owned {
				old["local/"+path] = localFile{Path: path}
			}
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	var sourceStaging string
	if item.Work.Kind == documentItem {
		raw, err := readItemRows(staging, item)
		if err != nil {
			return nil, err
		}
		var doc documentData
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, err
		}
		var metadata struct {
			Staging string `json:"staging_filename"`
		}
		if err := json.Unmarshal(doc.Metadata, &metadata); err != nil {
			return nil, err
		}
		sourceStaging = metadata.Staging
	}

	var originalSeen bool
	for _, file := range item.Files {
		var target string
		switch {
		case file.Name == "original":
			if item.Work.Kind != documentItem {
				return nil, errors.New("original file appears in processor item")
			}
			originalSeen = true
			target = pathutil.ResolveDataHomePath(d.Original)
		case strings.HasPrefix(file.Name, "result."):
			if item.Work.Kind != documentItem {
				return nil, errors.New("line file appears in processor item")
			}
			target = pathutil.ResolveDataHomePath(d.Result)
		default:
			name := strings.TrimPrefix(file.Name, "artifacts/")
			if item.Work.Kind == documentItem {
				from := docprocessing.SyncArtifactBaseName(sourceStaging, d.Parser)
				switch {
				case strings.HasPrefix(name, from):
					name = docprocessing.SyncArtifactBaseName(d.Staging, d.Parser) + strings.TrimPrefix(name, from)
				case strings.HasPrefix(name, filenameStem(sourceStaging)):
					name = filenameStem(d.Staging) + strings.TrimPrefix(name, filenameStem(sourceStaging))
				}
			} else {
				// These document processors use the artifact base plus an extension.
				ext := filepath.Ext(name)
				if item.Work.Kind != "structure_analyzer" && ext != ".txt" {
					name = docprocessing.SyncArtifactBaseName(d.Staging, d.Parser) + ext
				}
			}

			target = filepath.Join(dir, name)
		}
		if target == "" {
			return nil, errors.New("archive artifact has no local destination")
		}
		for _, existing := range files {
			if existing.Path == target {
				return nil, fmt.Errorf("multiple artifacts target %q", filepath.Base(target))
			}
		}
		files[file.Name] = localFile{Path: target, SHA256: file.SHA256}
		// A matching local input always keeps its original bytes and path.
		if file.Name == "original" {
			f, err := describeFile(file.Name, target, item.Work.Hash)
			if err == nil && f.SHA256 == file.SHA256 {
				continue
			}
			if d.Origin != "" {
				return nil, errors.New("existing original differs from source")
			}
		}
		if file.Omit {
			f, err := describeFile(file.Name, target, "")
			if err != nil || f.SHA256 != file.SHA256 {
				return nil, fmt.Errorf("omitted artifact %q changed locally", file.Name)
			}
			continue
		}
		src := filepath.Join(staging, filepath.FromSlash(archivePrefix(item.Work)+"/files/"+file.Name))
		if file.Name == "original" {
			if _, err := describeFile(file.Name, src, item.Work.Hash); err != nil {
				return nil, err
			}
		}
		if work := item.Work; artifactNeedsIDRewrite(work.Kind, file.Name) {
			var err error
			src, err = rewritePortableFile(src, work.SourceID, d.ID, work.Kind, d.Original)
			if err != nil {
				return nil, err
			}
		}
		journal.Changes = append(journal.Changes, fileChange{Target: target, Source: src})
	}
	if item.Work.Kind == documentItem && !originalSeen {
		return nil, errors.New("document item has no original file")
	}
	keep := map[string]bool{pathutil.ResolveDataHomePath(d.Original): true}
	for _, file := range files {
		keep[file.Path] = true
	}
	for _, file := range old {
		if file.Path != "" && !keep[file.Path] {
			keep[file.Path] = true
			journal.Changes = append(journal.Changes, fileChange{Target: file.Path})
		}
	}
	return files, nil
}
