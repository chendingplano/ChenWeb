package docbuilder

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound    = errors.New("not found")
	ErrInUse       = errors.New("document belongs to other groups")
	ErrUnsupported = errors.New("unsupported document type")
	ErrBadPath     = errors.New("invalid path")
)

// Store is the document registry and the group model, on the project database.
type Store struct {
	DB         *sql.DB
	Root       string                                           // absolute DOCUMENT_ROOT_DIR
	ArchiveDir string                                           // absolute; deleted documents are moved here
	PDFDir     string                                           // absolute; cached PDF exports
	NextID     func(ctx context.Context) (int64, error)         // shared ID manager, sequence IDSequence
	Convert    func(ctx context.Context, src, dst string) error // Markdown → PDF; nil uses pandoc
	Logf       func(format string, args ...any)

	syncMu sync.Mutex
}

// Member is one member of a group: a directory (recursive) or a file, relative to the root.
type Member struct {
	ID   int64  `json:"member_id"`
	Type string `json:"member_type"` // "dir" or "file"
	Path string `json:"path"`
}

type fileRow struct {
	FileID  int64
	DocID   int64
	Path    string
	Version string
	Hash    string
}

// docInfo is a registered document with its present (not missing) files.
type docInfo struct {
	ID    int64
	Kind  string
	Title string
	Files []fileRow
	Cur   fileRow // highest version
}

func (s *Store) logf(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
	}
}

// CleanRel validates a root-relative path: slash-separated, no "..", not absolute, no hidden
// segment, not empty. It returns the cleaned path.
func CleanRel(p string) (string, error) {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if p == "" || strings.HasPrefix(p, "/") || filepath.IsAbs(p) {
		return "", ErrBadPath
	}
	c := path.Clean(p)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") {
		return "", ErrBadPath
	}
	for _, seg := range strings.Split(c, "/") {
		if strings.HasPrefix(seg, ".") {
			return "", ErrBadPath
		}
	}
	return c, nil
}

func (s *Store) abs(rel string) string { return filepath.Join(s.Root, filepath.FromSlash(rel)) }

// covers reports whether any member of a group contains one of the document's files.
func covers(members []Member, d *docInfo) bool {
	for _, f := range d.Files {
		for _, m := range members {
			if memberCovers(m, f.Path) {
				return true
			}
		}
	}
	return false
}

func memberCovers(m Member, p string) bool {
	if m.Type == "dir" {
		return strings.HasPrefix(p, m.Path+"/")
	}
	mk, _ := ParsePath(m.Path)
	pk, _ := ParsePath(p)
	return path.Dir(p) == path.Dir(m.Path) && pk == mk
}

// ---- groups and members ----

type Group struct {
	ID      int64             `json:"group_id"`
	Key     string            `json:"group_key"`
	Label   map[string]string `json:"label"`
	Parent  string            `json:"parent_menu_id"`
	Order   int               `json:"sort_order"`
	Roles   []string          `json:"access_role"`
	Enabled bool              `json:"enabled"`
}

const groupCols = `group_id, group_key, label, parent_menu_id, sort_order, access_role, enabled`

type scanner interface{ Scan(dest ...any) error }

func scanGroup(r scanner) (*Group, error) {
	var g Group
	var label, roles []byte
	if err := r.Scan(&g.ID, &g.Key, &label, &g.Parent, &g.Order, &roles, &g.Enabled); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(label, &g.Label)
	_ = json.Unmarshal(roles, &g.Roles)
	return &g, nil
}

func (s *Store) GroupByKey(ctx context.Context, key string) (*Group, error) {
	g, err := scanGroup(s.DB.QueryRowContext(ctx, `SELECT `+groupCols+` FROM kb.doc_groups WHERE group_key=$1`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return g, err
}

func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+groupCols+` FROM kb.doc_groups ORDER BY parent_menu_id, sort_order, group_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

// SaveGroup inserts (ID 0) or updates a group.
func (s *Store) SaveGroup(ctx context.Context, g *Group, actor string) error {
	g.Key = strings.TrimSpace(g.Key)
	if g.Key == "" || strings.ContainsAny(g.Key, " /?#") {
		return fmt.Errorf("group_key must be a non-empty token without spaces or slashes")
	}
	if g.Parent == "" {
		g.Parent = "documents"
	}
	label, _ := json.Marshal(g.Label)
	roles, _ := json.Marshal(g.Roles)
	if g.ID == 0 {
		return s.DB.QueryRowContext(ctx, `INSERT INTO kb.doc_groups (group_key,label,parent_menu_id,sort_order,access_role,enabled,created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING group_id`, g.Key, label, g.Parent, g.Order, roles, g.Enabled, actor).Scan(&g.ID)
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE kb.doc_groups SET group_key=$2,label=$3,parent_menu_id=$4,sort_order=$5,access_role=$6,enabled=$7,updated_at=NOW() WHERE group_id=$1`,
		g.ID, g.Key, label, g.Parent, g.Order, roles, g.Enabled)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteGroup(ctx context.Context, groupID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM kb.doc_groups WHERE group_id=$1`, groupID)
	return err
}

func (s *Store) Members(ctx context.Context, groupID int64) ([]Member, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT member_id, member_type, path FROM kb.doc_group_members WHERE group_id=$1 ORDER BY member_id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.Type, &m.Path); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddMember adds a directory or file under the root to a group, registers its documents, and
// clears exclusions of the documents it brings in.
func (s *Store) AddMember(ctx context.Context, groupID int64, typ, rel string) (*Member, error) {
	rel, err := CleanRel(rel)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(s.abs(rel))
	if err != nil {
		return nil, fmt.Errorf("%w: %s does not exist under the document root", ErrBadPath, rel)
	}
	if typ != "dir" && typ != "file" || (typ == "dir") != fi.IsDir() {
		return nil, fmt.Errorf("%w: member_type %q does not match %s", ErrBadPath, typ, rel)
	}
	if typ == "file" && kindOf(rel) == "" {
		return nil, ErrUnsupported
	}
	m := Member{Type: typ, Path: rel}
	err = s.DB.QueryRowContext(ctx, `INSERT INTO kb.doc_group_members (group_id, member_type, path) VALUES ($1,$2,$3)
		ON CONFLICT (group_id, member_type, path) DO UPDATE SET path=EXCLUDED.path RETURNING member_id`, groupID, typ, rel).Scan(&m.ID)
	if err != nil {
		return nil, err
	}
	if err := s.SyncGroup(ctx, groupID); err != nil {
		return &m, err
	}
	docs, err := s.loadDocs(ctx)
	if err != nil {
		return &m, err
	}
	for _, d := range docs {
		if covers([]Member{m}, d) {
			if _, err := s.DB.ExecContext(ctx, `DELETE FROM kb.doc_group_exclusions WHERE group_id=$1 AND doc_id=$2`, groupID, d.ID); err != nil {
				return &m, err
			}
		}
	}
	return &m, nil
}

func (s *Store) RemoveMember(ctx context.Context, groupID, memberID int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.doc_group_members WHERE group_id=$1 AND member_id=$2`, groupID, memberID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- registry ----

// loadDocs loads every registered document with its present files and picks the latest version.
func (s *Store) loadDocs(ctx context.Context) (map[int64]*docInfo, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT f.file_id, f.doc_id, f.path, f.version, f.hash, d.kind, d.title
		FROM kb.doc_files f JOIN kb.doc_documents d USING (doc_id) WHERE NOT f.missing ORDER BY f.path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	docs := map[int64]*docInfo{}
	for rows.Next() {
		var f fileRow
		var kind, title string
		if err := rows.Scan(&f.FileID, &f.DocID, &f.Path, &f.Version, &f.Hash, &kind, &title); err != nil {
			return nil, err
		}
		d := docs[f.DocID]
		if d == nil {
			d = &docInfo{ID: f.DocID, Kind: kind, Title: title}
			docs[f.DocID] = d
		}
		d.Files = append(d.Files, f)
		if len(d.Files) == 1 || Newer(f.Version, d.Cur.Version) {
			d.Cur = f
		}
	}
	return docs, rows.Err()
}

type found struct {
	size, mtime int64
}

// walkMember lists the supported files a member currently has on disk.
func (s *Store) walkMember(m Member, out map[string]found) {
	add := func(rel string, fi fs.FileInfo) {
		if kindOf(rel) != "" {
			out[rel] = found{fi.Size(), fi.ModTime().UnixNano()}
		}
	}
	if m.Type == "file" {
		entries, err := os.ReadDir(s.abs(path.Dir(m.Path)))
		if err != nil {
			return
		}
		for _, e := range entries {
			rel := path.Join(path.Dir(m.Path), e.Name())
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") || !memberCovers(m, rel) {
				continue
			}
			if fi, err := e.Info(); err == nil {
				add(rel, fi)
			}
		}
		return
	}
	root := s.abs(m.Path)
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p != root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.Root, p)
		if err != nil {
			return nil
		}
		if fi, err := d.Info(); err == nil {
			add(filepath.ToSlash(rel), fi)
		}
		return nil
	})
}

// SyncGroup brings the registry in line with the group's members on disk:
// known paths refresh their hash (a changed file loses its PDF export); an unknown path whose
// content matches a file that vanished is a move (same document ID); an unknown path that shares a
// logical key with a document is a new version of it; anything else is a new document. Files that
// are gone are marked missing.
func (s *Store) SyncGroup(ctx context.Context, groupID int64) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	members, err := s.Members(ctx, groupID)
	if err != nil {
		return err
	}
	onDisk := map[string]found{}
	for _, m := range members {
		s.walkMember(m, onDisk)
	}
	rels := make([]string, 0, len(onDisk))
	for rel := range onDisk {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	touched := map[int64]bool{}

	for _, rel := range rels {
		fi := onDisk[rel]
		var fileID, docID, size, mtime int64
		var hash string
		var missing bool
		err := s.DB.QueryRowContext(ctx, `SELECT file_id, doc_id, hash, size_bytes, mtime_ns, missing FROM kb.doc_files WHERE path=$1`, rel).
			Scan(&fileID, &docID, &hash, &size, &mtime, &missing)
		switch {
		case err == nil:
			if !missing && size == fi.size && mtime == fi.mtime {
				continue
			}
			h, herr := hashFile(s.abs(rel))
			if herr != nil {
				continue
			}
			if _, err := s.DB.ExecContext(ctx, `UPDATE kb.doc_files SET hash=$2,size_bytes=$3,mtime_ns=$4,missing=FALSE,updated_at=NOW() WHERE file_id=$1`,
				fileID, h, fi.size, fi.mtime); err != nil {
				return err
			}
			if h != hash {
				s.dropExport(ctx, fileID)
			}
			touched[docID] = true
		case errors.Is(err, sql.ErrNoRows):
			if err := s.registerNew(ctx, rel, fi, onDisk, touched); err != nil {
				return err
			}
		default:
			return err
		}
	}

	// Files that were present and are gone.
	rows, err := s.DB.QueryContext(ctx, `SELECT file_id, doc_id, path FROM kb.doc_files WHERE NOT missing`)
	if err != nil {
		return err
	}
	type gone struct {
		fileID, docID int64
	}
	var goneFiles []gone
	for rows.Next() {
		var fid, did int64
		var p string
		if err := rows.Scan(&fid, &did, &p); err != nil {
			rows.Close()
			return err
		}
		if _, ok := onDisk[p]; ok {
			continue
		}
		inScope := false
		for _, m := range members {
			if memberCovers(m, p) {
				inScope = true
			}
		}
		if _, err := os.Stat(s.abs(p)); inScope && errors.Is(err, fs.ErrNotExist) {
			goneFiles = append(goneFiles, gone{fid, did})
		}
	}
	rows.Close()
	for _, g := range goneFiles {
		if _, err := s.DB.ExecContext(ctx, `UPDATE kb.doc_files SET missing=TRUE,updated_at=NOW() WHERE file_id=$1`, g.fileID); err != nil {
			return err
		}
		s.dropExport(ctx, g.fileID)
		touched[g.docID] = true
	}

	if err := s.refreshTitles(ctx, touched); err != nil {
		return err
	}
	return s.importLegacy(ctx, groupID, members)
}

func (s *Store) registerNew(ctx context.Context, rel string, fi found, onDisk map[string]found, touched map[int64]bool) error {
	h, err := hashFile(s.abs(rel))
	if err != nil {
		return nil // unreadable now; try again next sync
	}
	key, ver := ParsePath(rel)

	// A move: the same content used to live at a path that is gone.
	rows, err := s.DB.QueryContext(ctx, `SELECT file_id, doc_id, path FROM kb.doc_files WHERE hash=$1`, h)
	if err != nil {
		return err
	}
	type cand struct {
		fileID, docID int64
		path          string
	}
	var cands []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.fileID, &c.docID, &c.path); err != nil {
			rows.Close()
			return err
		}
		cands = append(cands, c)
	}
	rows.Close()
	for _, c := range cands {
		if _, ok := onDisk[c.path]; ok {
			continue
		}
		if _, err := os.Stat(s.abs(c.path)); err == nil {
			continue
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE kb.doc_files SET path=$2,version=$3,size_bytes=$4,mtime_ns=$5,missing=FALSE,updated_at=NOW() WHERE file_id=$1`,
			c.fileID, rel, ver, fi.size, fi.mtime); err != nil {
			return err
		}
		var n int
		_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM kb.doc_files WHERE doc_id=$1`, c.docID).Scan(&n)
		if n == 1 {
			if _, err := s.DB.ExecContext(ctx, `UPDATE kb.doc_documents SET logical_key=$2 WHERE doc_id=$1`, c.docID, key); err != nil {
				return err
			}
		}
		touched[c.docID] = true
		return nil
	}

	// A new version of a known document, or a new document.
	var docID int64
	err = s.DB.QueryRowContext(ctx, `SELECT doc_id FROM kb.doc_documents WHERE logical_key=$1 ORDER BY doc_id LIMIT 1`, key).Scan(&docID)
	if errors.Is(err, sql.ErrNoRows) {
		if s.NextID == nil {
			return errors.New("docbuilder: no ID allocator configured")
		}
		if docID, err = s.NextID(ctx); err != nil || docID <= 0 {
			return fmt.Errorf("allocate document ID: %v", err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO kb.doc_documents (doc_id, logical_key, kind) VALUES ($1,$2,$3)`, docID, key, kindOf(rel)); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO kb.doc_files (doc_id, path, version, hash, size_bytes, mtime_ns) VALUES ($1,$2,$3,$4,$5,$6)`,
		docID, rel, ver, h, fi.size, fi.mtime); err != nil {
		return err
	}
	touched[docID] = true
	return nil
}

// refreshTitles sets each touched document's title from its current (latest) file.
func (s *Store) refreshTitles(ctx context.Context, touched map[int64]bool) error {
	if len(touched) == 0 {
		return nil
	}
	docs, err := s.loadDocs(ctx)
	if err != nil {
		return err
	}
	for id := range touched {
		d := docs[id]
		if d == nil {
			continue
		}
		t := titleOf(s.abs(d.Cur.Path), d.Kind)
		if _, err := s.DB.ExecContext(ctx, `UPDATE kb.doc_documents SET title=$2 WHERE doc_id=$1`, id, t); err != nil {
			return err
		}
	}
	return nil
}

// importLegacy copies categories saved by the directory-based builder
// (<dir>/.docbuilder/manifest.json) into the group, once per group, so nothing is classified twice.
func (s *Store) importLegacy(ctx context.Context, groupID int64, members []Member) error {
	var done bool
	if err := s.DB.QueryRowContext(ctx, `SELECT legacy_imported FROM kb.doc_groups WHERE group_id=$1`, groupID).Scan(&done); err != nil || done {
		return err
	}
	for _, m := range members {
		if m.Type != "dir" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.abs(m.Path), ".docbuilder", "manifest.json"))
		if err != nil {
			continue
		}
		var legacy struct {
			Entries []struct{ Path, Category string } `json:"entries"`
		}
		if json.Unmarshal(raw, &legacy) != nil {
			continue
		}
		for _, e := range legacy.Entries {
			if strings.TrimSpace(e.Category) == "" {
				continue
			}
			if _, err := s.DB.ExecContext(ctx, `INSERT INTO kb.doc_group_categories (group_id, doc_id, category, model)
				SELECT $1, doc_id, $3, 'legacy-manifest' FROM kb.doc_files WHERE path=$2
				ON CONFLICT (group_id, doc_id) DO NOTHING`, groupID, m.Path+"/"+e.Path, cleanCategory(e.Category)); err != nil {
				return err
			}
		}
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE kb.doc_groups SET legacy_imported=TRUE WHERE group_id=$1`, groupID)
	return err
}

// ---- reading a group ----

// GroupDocs returns the documents of a group (members minus exclusions), each with its latest file
// and the group's category for it.
func (s *Store) GroupDocs(ctx context.Context, groupID int64) ([]Entry, error) {
	members, err := s.Members(ctx, groupID)
	if err != nil {
		return nil, err
	}
	docs, err := s.loadDocs(ctx)
	if err != nil {
		return nil, err
	}
	excl, err := s.idSet(ctx, `SELECT doc_id FROM kb.doc_group_exclusions WHERE group_id=$1`, groupID)
	if err != nil {
		return nil, err
	}
	cats := map[int64]string{}
	rows, err := s.DB.QueryContext(ctx, `SELECT doc_id, category FROM kb.doc_group_categories WHERE group_id=$1`, groupID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var c string
		if err := rows.Scan(&id, &c); err != nil {
			rows.Close()
			return nil, err
		}
		cats[id] = c
	}
	rows.Close()
	var out []Entry
	for id, d := range docs {
		if excl[id] || !covers(members, d) {
			continue
		}
		out = append(out, Entry{DocID: id, Path: d.Cur.Path, Kind: d.Kind, Title: d.Title, Category: cats[id]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DocID < out[j].DocID })
	return out, nil
}

func (s *Store) idSet(ctx context.Context, q string, args ...any) (map[int64]bool, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// CurrentFile returns the latest file of a document if it belongs to the group.
func (s *Store) CurrentFile(ctx context.Context, groupID, docID int64) (path, kind string, err error) {
	entries, err := s.GroupDocs(ctx, groupID)
	if err != nil {
		return "", "", err
	}
	for _, e := range entries {
		if e.DocID == docID {
			return e.Path, e.Kind, nil
		}
	}
	return "", "", ErrNotFound
}

// ---- classification ----

// ClassifyGroup categorises the group's documents that have no category (all of them with force).
func (s *Store) ClassifyGroup(ctx context.Context, g *Group, c Classifier, model string, force bool) (int, error) {
	if err := s.SyncGroup(ctx, g.ID); err != nil {
		return 0, err
	}
	if force {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM kb.doc_group_categories WHERE group_id=$1`, g.ID); err != nil {
			return 0, err
		}
	}
	entries, err := s.GroupDocs(ctx, g.ID)
	if err != nil {
		return 0, err
	}
	var existing []string
	seen := map[string]bool{}
	var items []Candidate
	byPath := map[string]int64{}
	for _, e := range entries {
		if e.Category != "" {
			if !seen[e.Category] {
				seen[e.Category] = true
				existing = append(existing, e.Category)
			}
			continue
		}
		ex := ""
		if e.Kind == KindMarkdown {
			if raw, err := os.ReadFile(s.abs(e.Path)); err == nil {
				ex = Excerpt(string(raw))
			}
		}
		items = append(items, Candidate{Path: e.Path, Title: e.Title, Excerpt: ex})
		byPath[e.Path] = e.DocID
	}
	return ClassifyBatches(ctx, existing, items, c, func(answers map[string]string) error {
		for p, cat := range answers {
			if _, err := s.DB.ExecContext(ctx, `INSERT INTO kb.doc_group_categories (group_id, doc_id, category, model)
				VALUES ($1,$2,$3,$4) ON CONFLICT (group_id, doc_id) DO UPDATE SET category=EXCLUDED.category, model=EXCLUDED.model, classified_at=NOW()`,
				g.ID, byPath[p], cat, model); err != nil {
				return err
			}
		}
		return nil
	})
}

// ---- removal, deletion, archive ----

// RemoveDocument removes a document from one group: it is excluded from the group (and any file
// member for it is dropped). With del it is also deleted from disk, unless another group contains
// it: then nothing changes, ErrInUse is returned and blockedBy names those groups. Deleting moves
// every present version to the archive and keeps the registry rows.
func (s *Store) RemoveDocument(ctx context.Context, groupID, docID int64, del bool) (blockedBy []string, err error) {
	docs, err := s.loadDocs(ctx)
	if err != nil {
		return nil, err
	}
	d := docs[docID]
	groups, err := s.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	in := func(gid int64) (bool, []Member, error) {
		members, err := s.Members(ctx, gid)
		if err != nil {
			return false, nil, err
		}
		excl, err := s.idSet(ctx, `SELECT doc_id FROM kb.doc_group_exclusions WHERE group_id=$1`, gid)
		if err != nil {
			return false, nil, err
		}
		return d != nil && !excl[docID] && covers(members, d), members, nil
	}
	ok, members, err := in(groupID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	if del {
		for _, g := range groups {
			if g.ID == groupID {
				continue
			}
			other, _, err := in(g.ID)
			if err != nil {
				return nil, err
			}
			if other {
				blockedBy = append(blockedBy, g.Key)
			}
		}
		if len(blockedBy) > 0 {
			return blockedBy, ErrInUse
		}
	}
	for _, m := range members {
		if m.Type == "file" && covers([]Member{m}, d) {
			if _, err := s.DB.ExecContext(ctx, `DELETE FROM kb.doc_group_members WHERE member_id=$1`, m.ID); err != nil {
				return nil, err
			}
		}
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO kb.doc_group_exclusions (group_id, doc_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, groupID, docID); err != nil {
		return nil, err
	}
	if del {
		return nil, s.archive(ctx, d)
	}
	return nil, nil
}

func (s *Store) archive(ctx context.Context, d *docInfo) error {
	stamp := time.Now().UTC().Format("20060102T150405Z")
	for _, f := range d.Files {
		dst := filepath.Join(s.ArchiveDir, stamp, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := moveFile(s.abs(f.Path), dst); err != nil {
			return err
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE kb.doc_files SET missing=TRUE, updated_at=NOW() WHERE file_id=$1`, f.FileID); err != nil {
			return err
		}
		s.dropExport(ctx, f.FileID)
	}
	return nil
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

// ---- PDF export ----

// ExportPDF returns the path of a PDF for a Markdown document's current version, converting only
// when no export exists for its current content. A PDF document is returned as it is.
func (s *Store) ExportPDF(ctx context.Context, groupID, docID int64) (string, error) {
	rel, kind, err := s.CurrentFile(ctx, groupID, docID)
	if err != nil {
		return "", err
	}
	if kind == KindPDF {
		return s.abs(rel), nil
	}
	if kind != KindMarkdown {
		return "", ErrUnsupported
	}
	var fileID int64
	var hash string
	if err := s.DB.QueryRowContext(ctx, `SELECT file_id, hash FROM kb.doc_files WHERE path=$1`, rel).Scan(&fileID, &hash); err != nil {
		return "", err
	}
	var srcHash, pdf string
	err = s.DB.QueryRowContext(ctx, `SELECT source_hash, pdf_path FROM kb.doc_pdf_exports WHERE file_id=$1`, fileID).Scan(&srcHash, &pdf)
	if err == nil && srcHash == hash {
		if _, err := os.Stat(pdf); err == nil {
			return pdf, nil
		}
	}
	dst := filepath.Join(s.PDFDir, strconv.FormatInt(fileID, 10)+".pdf")
	if err := os.MkdirAll(s.PDFDir, 0o755); err != nil {
		return "", err
	}
	convert := s.Convert
	if convert == nil {
		convert = PandocConvert("typst")
	}
	if err := convert(ctx, s.abs(rel), dst); err != nil {
		return "", err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO kb.doc_pdf_exports (file_id, source_hash, pdf_path) VALUES ($1,$2,$3)
		ON CONFLICT (file_id) DO UPDATE SET source_hash=EXCLUDED.source_hash, pdf_path=EXCLUDED.pdf_path, created_at=NOW()`, fileID, hash, dst)
	return dst, err
}

// dropExport deletes a file's stored PDF export (row and file), if any.
func (s *Store) dropExport(ctx context.Context, fileID int64) {
	var pdf string
	if err := s.DB.QueryRowContext(ctx, `SELECT pdf_path FROM kb.doc_pdf_exports WHERE file_id=$1`, fileID).Scan(&pdf); err != nil {
		return
	}
	_ = os.Remove(pdf)
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM kb.doc_pdf_exports WHERE file_id=$1`, fileID)
}

// PandocConvert converts Markdown to PDF with pandoc and the given PDF engine (for example "typst").
func PandocConvert(engine string) func(ctx context.Context, src, dst string) error {
	return func(ctx context.Context, src, dst string) error {
		cmd := exec.CommandContext(ctx, "pandoc", "-f", "markdown", src, "-o", dst, "--pdf-engine="+engine)
		if out, err := cmd.CombinedOutput(); err != nil {
			_ = os.Remove(dst)
			return fmt.Errorf("pandoc: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
}
