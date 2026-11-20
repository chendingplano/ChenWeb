package docsync

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/docsync/transfer"
)

func (s Source) pack(ctx context.Context, req transfer.PackRequest, output string) (transfer.PackResult, error) {
	var work []workItem
	if err := json.Unmarshal(req.Work, &work); err != nil || len(work) == 0 {
		return transfer.PackResult{}, errors.New("empty or invalid document sync work list")
	}
	manifest := manifest{Version: formatVersion, Exporter: "docsync-v1", RunID: req.RunID, TarNumber: req.TarNumber}
	staged, err := os.CreateTemp(filepath.Dir(output), ".items-*")
	if err != nil {
		return transfer.PackResult{}, err
	}
	defer os.Remove(staged.Name())
	defer staged.Close()
	var included, deferred []workItem
	for _, item := range work {
		if err := ctx.Err(); err != nil {
			return transfer.PackResult{}, err
		}
		start, err := staged.Seek(0, io.SeekCurrent)
		if err != nil {
			return transfer.PackResult{}, err
		}
		packed, err := s.packItem(ctx, staged, item)
		if err != nil {
			if truncateErr := staged.Truncate(start); truncateErr != nil {
				return transfer.PackResult{}, errors.Join(err, truncateErr)
			}
			if _, seekErr := staged.Seek(start, io.SeekStart); seekErr != nil {
				return transfer.PackResult{}, errors.Join(err, seekErr)
			}
			item.Issue = err.Error()
			deferred = append(deferred, item)
			continue
		}
		manifest.Items = append(manifest.Items, packed)
		included = append(included, item)
		position, err := staged.Seek(0, io.SeekCurrent)
		if err != nil {
			return transfer.PackResult{}, err
		}
		if position >= req.MaxBytes {
			break
		}
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return transfer.PackResult{}, err
	}
	f, err := os.Create(output)
	if err != nil {
		return transfer.PackResult{}, err
	}
	err = writeArchive(f, manifest, staged)
	if closeErr := f.Close(); closeErr != nil {
		err = errors.Join(err, closeErr)
	}
	if err != nil {
		return transfer.PackResult{}, err
	}
	sent, err := json.Marshal(included)
	if err != nil {
		return transfer.PackResult{}, err
	}
	skipped, err := json.Marshal(deferred)
	return transfer.PackResult{Included: sent, Deferred: skipped}, err
}

// Each item is a gzip member containing tar entries without the end markers.
// This permits measuring compressed item sizes before writing the first entry.
func writeArchive(dst io.Writer, m manifest, items io.Reader) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(dst)
	tw := tar.NewWriter(gz)
	if err := writeTarFile(tw, "manifest.json", int64(len(raw)), bytes.NewReader(raw)); err != nil {
		return err
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if _, err := io.Copy(dst, items); err != nil {
		return err
	}
	gz = gzip.NewWriter(dst)
	tw = tar.NewWriter(gz)
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func (s Source) packItem(ctx context.Context, out io.Writer, item workItem) (manifestItem, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return manifestItem{}, err
	}
	defer tx.Rollback()
	var input sourceInput
	if item.Kind != VocabularyProcessor {
		input, err = readSourceInput(ctx, tx, item.SourceID)
		if err != nil {
			return manifestItem{}, err
		}
		if input.hash != item.Hash || input.duplicate {
			return manifestItem{}, errors.New("source document identity changed before packing")
		}
		winner, err := sourceWinner(ctx, tx, item.Hash)
		if err != nil || winner != item.SourceID {
			return manifestItem{}, errors.New("source duplicate selection changed before packing")
		}
		if !bytes.Equal(input.status[item.Kind], item.Status) {
			return manifestItem{}, errors.New("source status changed before packing")
		}
	}
	snap, err := s.snapshot(ctx, tx, input, item)
	if err != nil {
		return manifestItem{}, err
	}
	if snap.item.Digest != item.Digest {
		return manifestItem{}, errors.New("source output changed before packing")
	}
	m := manifestItem{Work: item, Rows: snap.rows, Contract: bundleContractVersion}
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	prefix := archivePrefix(item)
	m.Data, err = writeItemRows(tw, prefix, snap)
	if err != nil {
		return manifestItem{}, err
	}

	for _, file := range snap.files {
		meta := file.fileEntry
		meta.Omit = item.Held[file.Name] == file.SHA256
		m.Files = append(m.Files, meta)
		if !meta.Omit {
			if err := writeSourceArtifact(tw, prefix+"/files/"+file.Name, file); err != nil {
				return manifestItem{}, err
			}
		}
	}
	if err := tw.Flush(); err != nil {
		return manifestItem{}, err
	}
	if err := gz.Close(); err != nil {
		return manifestItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return manifestItem{}, err
	}
	if item.Kind != VocabularyProcessor {
		var observed []byte
		if err := s.DB.QueryRowContext(ctx, `SELECT status FROM kb.inputs WHERE id=$1`, item.SourceID).Scan(&observed); err != nil {
			return manifestItem{}, err
		}
		var entries []json.RawMessage
		if err := json.Unmarshal(observed, &entries); err != nil {
			return manifestItem{}, err
		}
		current := make(map[string]json.RawMessage)
		for _, entry := range entries {
			var st processorStatus
			if err := json.Unmarshal(entry, &st); err != nil {
				return manifestItem{}, err
			}
			for _, key := range statusKeys(st.Operation) {
				normalized, err := compactStatus(entry)
				if err != nil {
					return manifestItem{}, err
				}
				current[key] = normalized
			}
		}
		keys := []string{item.Kind}
		if item.Kind == documentItem {
			keys = []string{"parsed", "converted", "static_analyzer"}
		}
		for _, key := range keys {
			if !bytes.Equal(current[key], input.status[key]) {
				return manifestItem{}, errors.New("source status changed during packing")
			}
		}
	}
	return m, nil
}

func writeItemRows(tw *tar.Writer, prefix string, snap snapshot) ([]rowFile, error) {
	write := func(name string, count int, data []byte) (rowFile, error) {
		meta := rowFile{fileEntry: fileEntry{Name: name, Size: int64(len(data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}, Rows: count}
		return meta, writeTarFile(tw, prefix+"/"+name, meta.Size, bytes.NewReader(data))
	}
	if bundle, ok := snap.data.(Bundle); ok {
		tables := make([]string, 0, len(bundle.Rows))
		for table := range bundle.Rows {
			tables = append(tables, table)
		}
		slices.Sort(tables)
		files := make([]rowFile, 0, len(tables))
		for _, table := range tables {
			var buffer bytes.Buffer
			enc := json.NewEncoder(&buffer)
			for _, row := range bundle.Rows[table] {
				if err := enc.Encode(row); err != nil {
					return nil, err
				}
			}
			file, err := write(table+".jsonl", len(bundle.Rows[table]), buffer.Bytes())
			if err != nil {
				return nil, err
			}
			files = append(files, file)
		}
		return files, nil
	}
	name := "status.json"
	if snap.item.Kind == documentItem {
		name = "input.json"
	}
	raw, err := json.Marshal(snap.data)
	if err != nil {
		return nil, err
	}
	file, err := write(name, snap.rows, raw)
	return []rowFile{file}, err
}

func archivePrefix(item workItem) string {
	return fmt.Sprintf("%d/%s", item.SourceID, item.Kind)
}

func writeSourceArtifact(tw *tar.Writer, name string, file sourceFile) error {
	src, err := os.Open(file.path)
	if err != nil {
		return err
	}
	defer src.Close()
	hash := sha256.New()
	if err := writeTarFile(tw, name, file.Size, io.TeeReader(src, hash)); err != nil {
		return err
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != file.SHA256 {
		return errors.New("source file changed during packing")
	}
	return nil
}

func writeTarFile(tw *tar.Writer, name string, size int64, data io.Reader) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Size: size, Mode: 0o600, Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	n, err := io.Copy(tw, data)
	if err == nil && n != size {
		err = io.ErrUnexpectedEOF
	}
	return err
}

func unpackArchive(file, staging string, runID int64, tarNumber int, expected []workItem) (manifest, error) {
	var m manifest
	f, err := os.Open(file)
	if err != nil {
		return m, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return m, err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	header, err := reader.Next()
	if err != nil {
		return m, err
	}
	if header.Name != "manifest.json" || header.Typeflag != tar.TypeReg || header.Size > 32<<20 {
		return m, errors.New("invalid manifest entry")
	}
	if err := json.NewDecoder(reader).Decode(&m); err != nil {
		return m, err
	}
	if m.Version != formatVersion {
		return m, fmt.Errorf("unsupported document sync format version %d", m.Version)
	}
	if m.RunID != runID || m.TarNumber != tarNumber || len(m.Items) != len(expected) {
		return m, errors.New("manifest does not match requested archive")
	}
	paths := make(map[string]fileEntry)
	for i, item := range m.Items {
		if !sameWork(item.Work, expected[i]) {
			return m, errors.New("manifest has an unrequested item or a conflicting digest")
		}
		prefix := archivePrefix(item.Work)
		if item.Contract != bundleContractVersion {
			return m, fmt.Errorf("unsupported %s contract version %d", item.Work.Kind, item.Contract)
		}
		for _, file := range item.Data {
			name := prefix + "/" + file.Name
			if !validRowFile(item.Work.Kind, file.Name) || file.Omit || file.Rows < 0 {
				return m, fmt.Errorf("invalid row manifest entry %q", file.Name)
			}
			if _, exists := paths[name]; exists {
				return m, fmt.Errorf("duplicate row manifest entry %q", file.Name)
			}
			paths[name] = file.fileEntry
		}
		for _, f := range item.Files {
			if !validArchiveName(f.Name) || !validFileRole(f.Name) {
				return m, fmt.Errorf("invalid artifact path %q", f.Name)
			}
			if f.Omit {
				if item.Work.Held[f.Name] != f.SHA256 {
					return m, errors.New("export omitted an unrecognized file")
				}
				continue
			}
			name := prefix + "/files/" + f.Name
			if _, exists := paths[name]; exists {
				return m, fmt.Errorf("duplicate manifest artifact %q", f.Name)
			}
			paths[name] = f
		}
	}
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return m, err
		}
		meta, found := paths[header.Name]
		if !found || !validArchiveName(header.Name) || header.Typeflag != tar.TypeReg || header.Size != meta.Size {
			return m, fmt.Errorf("unexpected archive entry %q", header.Name)
		}
		target := filepath.Join(staging, filepath.FromSlash(header.Name))
		if err := extractFile(target, reader, meta); err != nil {
			return m, err
		}
		delete(paths, header.Name)
	}
	if len(paths) != 0 {
		return m, errors.New("archive is missing manifest entries")
	}
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return m, err
	}
	for _, item := range m.Items {
		if _, err := readItemRows(staging, item); err != nil {
			return m, err
		}
	}
	return m, nil
}

func sameWork(a, b workItem) bool {
	return a.SourceID == b.SourceID && a.Kind == b.Kind && a.Hash == b.Hash &&
		a.Digest == b.Digest && a.SeenAt.Equal(b.SeenAt) &&
		bytes.Equal(a.Status, b.Status) && reflect.DeepEqual(a.Requires, b.Requires) && reflect.DeepEqual(a.Held, b.Held)
}

func validArchiveName(name string) bool {
	return name != "." && name != "" && path.Clean(name) == name &&
		!strings.HasPrefix(name, "/") && !strings.HasPrefix(name, "../") && !strings.ContainsAny(name, "\\:\x00")
}

func validFileRole(name string) bool {
	return name == "original" ||
		(strings.HasPrefix(name, "result.") && path.Base(name) == name) ||
		(strings.HasPrefix(name, "artifacts/") && (path.Dir(name) == "artifacts" || strings.HasPrefix(name, "artifacts/images/")))
}

func extractFile(target string, r io.Reader, meta fileEntry) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hash), r)
	err = errors.Join(err, f.Close())
	if err == nil && (n != meta.Size || fmt.Sprintf("%x", hash.Sum(nil)) != meta.SHA256) {
		err = fmt.Errorf("artifact %s does not match manifest checksum", filepath.Base(target))
	}
	return err
}

func validRowFile(kind, name string) bool {
	switch kind {
	case documentItem:
		return name == "input.json"
	case pipelineItem:
		return name == "status.json"
	default:
		table, ok := strings.CutSuffix(name, ".jsonl")
		if !ok || !validArchiveName(name) {
			return false
		}
		// Table names are local contract identifiers, with support for
		// additive tables understood only by a later minor exporter.
		for _, char := range table {
			if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_') {
				return false
			}
		}
		return table != ""
	}
}

func readItemRows(staging string, item manifestItem) (json.RawMessage, error) {
	dir := filepath.Join(staging, filepath.FromSlash(archivePrefix(item.Work)))
	switch item.Work.Kind {
	case documentItem, pipelineItem:
		if len(item.Data) != 1 || item.Data[0].Rows != item.Rows || !validRowFile(item.Work.Kind, item.Data[0].Name) {
			return nil, errors.New("invalid item data manifest")
		}
		raw, err := os.ReadFile(filepath.Join(dir, item.Data[0].Name))
		if err != nil {
			return nil, err
		}
		if item.Work.Kind == documentItem {
			var doc documentData
			if err := json.Unmarshal(raw, &doc); err != nil {
				return nil, err
			}
			if len(doc.Metadata) == 0 || item.Rows != 1 {
				return nil, errors.New("document metadata is missing")
			}
		} else if !json.Valid(raw) || item.Rows != 0 {
			return nil, errors.New("invalid pipeline summary")
		}
		return raw, nil
	default:
		bundle := Bundle{Rows: make(map[string][]PortableRow)}
		var count int
		for _, file := range item.Data {
			rows, err := readTableRows(filepath.Join(dir, file.Name))
			if err != nil {
				return nil, err
			}
			if len(rows) != file.Rows {
				return nil, fmt.Errorf("row count does not match manifest for %s", file.Name)
			}
			count += len(rows)
			bundle.Rows[strings.TrimSuffix(file.Name, ".jsonl")] = rows
		}
		if count != item.Rows {
			return nil, errors.New("archive item row count does not match manifest")
		}
		return json.Marshal(bundle)
	}
}

func readTableRows(path string) ([]PortableRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var rows []PortableRow
	for {
		var row PortableRow
		err := dec.Decode(&row)
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
}
