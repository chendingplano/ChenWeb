package docbenchmark

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrUnsafePath        = errors.New("benchmark workspace: unsafe path")
	ErrVerifiedImmutable = errors.New("benchmark evidence is immutable")
)

type WorkspaceConfig struct {
	WorkRoot, EvidenceRoot, AttemptID, CaseID, RunID string
	Nonce                                            string
	Store                                            OwnershipStore
	Cleanup                                          func(CleanupTx) error
}
type AllocationMarker struct {
	AttemptID        string    `json:"attempt_id"`
	Nonce            string    `json:"nonce"`
	WorkRoot         string    `json:"work_root"`
	EvidenceRoot     string    `json:"evidence_root"`
	Workspace        string    `json:"workspace"`
	EvidencePath     string    `json:"evidence_path"`
	CreatedAt        time.Time `json:"created_at"`
	WorkIdentity     string    `json:"work_identity"`
	EvidenceIdentity string    `json:"evidence_identity"`
	ArtifactName     string    `json:"artifact_name,omitempty"`
}
type WorkspaceAllocation struct {
	Config                             WorkspaceConfig
	WorkPath, EvidencePath, MarkerPath string
	Marker                             AllocationMarker
	workRootFD, evidenceRootFD         *os.Root
	workDirFD, evidenceDirFD           *os.Root
	lastArtifact                       string
	mu                                 sync.Mutex
	closed                             bool
}

func relToRoot(root, path string) (string, error) {
	r, err := filepath.Rel(root, path)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || filepath.IsAbs(r) {
		return "", ErrUnsafePath
	}
	return filepath.ToSlash(r), nil
}
func mustRel(root, path string) string {
	r, err := relToRoot(root, path)
	if err != nil {
		return ""
	}
	return r
}

func writeMarkerAtomicRoot(root *os.Root, rel string, m AllocationMarker) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := rel + ".tmp"
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		_ = root.Remove(tmp)
		return err
	}
	if err = root.Rename(tmp, rel); err != nil {
		_ = root.Remove(tmp)
		return err
	}
	if d, e := root.Open(filepath.Dir(rel)); e == nil {
		e = d.Sync()
		_ = d.Close()
		return e
	} else {
		return e
	}
}

type Artifact struct {
	Path, SHA256 string
	SizeBytes    int64
	Verified     bool
	Metadata     map[string]string
}
type CaptureFailurePoint string

const (
	FailAfterPartialCreate   CaptureFailurePoint = "partial_create"
	FailAfterCopy            CaptureFailurePoint = "copy"
	FailAfterFileSync        CaptureFailurePoint = "file_sync"
	FailAfterRename          CaptureFailurePoint = "rename"
	FailAfterHash            CaptureFailurePoint = "hash"
	FailAfterDirectorySync   CaptureFailurePoint = "directory_sync"
	FailBeforeVerifiedCommit CaptureFailurePoint = "before_verified_commit"
)

type CaptureOptions struct{ Failure CaptureFailurePoint }

// CleanupOptions carries the database-side cleanup operation. It runs inside
// the ownership transaction and must be non-nil for cleanup to proceed.
type CleanupOptions struct {
	DiscardUnverified bool
	Cleanup           func(CleanupTx) error
}
type Ownership struct {
	AttemptID, Workspace, EvidencePath, Nonce, WorkRoot, EvidenceRoot string
	Verified                                                          bool
	VerifiedHash                                                      string
	VerifiedSize                                                      int64
	VerifiedMarker                                                    string
	CleanupState                                                      string
}
type OwnershipStore interface {
	LoadOwnership(attemptID string) (Ownership, error)
	SaveOwnership(Ownership) error
	LockOwnership(attemptID string) (Ownership, error)
	MarkVerified(attemptID, hash string, size int64, marker AllocationMarker) error
	MarkVerifiedCAS(ctx context.Context, owner, expectedNonce, expectedMarkerHash, hash string, size int64, marker AllocationMarker) error
	MarkCleanupState(attemptID, state string, cause error) error
	CleanupTransaction(ctx context.Context, owner string, fn func(CleanupTx) error) error
}

func writeMarkerAtomic(path string, m AllocationMarker) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err == nil {
		err = d.Sync()
		d.Close()
	}
	return err
}

// CleanupTx is the transaction-scoped cleanup capability for an attempt.
// Implementations bind all operations to the owner supplied to
// CleanupTransaction and roll them back when the callback returns an error.
type CleanupTx interface {
	DeleteProductionRows() error
	DeleteInput() error
	MarkState(state string, cause error) error
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}
func markerDigest(m AllocationMarker) string {
	b, _ := json.Marshal(m)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func safeComponent(s string) bool {
	if s == "" || s == "." || s == ".." || filepath.Base(s) != s {
		return false
	}
	for _, r := range s {
		if r == '/' || r == '\\' || r < 32 {
			return false
		}
	}
	return true
}
func canonicalRoot(p string) (string, error) {
	if p == "" {
		return "", ErrUnsafePath
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if err := mkdirNoSymlink(abs); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}
func mkdirNoSymlink(p string) error {
	abs, _ := filepath.Abs(p)
	if i, e := os.Lstat(abs); e == nil {
		if i.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink", ErrUnsafePath)
		}
		return nil
	}
	parent := filepath.Dir(abs)
	if err := mkdirNoSymlink(parent); err != nil {
		return err
	}
	if err := os.Mkdir(abs, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	i, e := os.Lstat(abs)
	if e != nil {
		return e
	}
	if i.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: symlink", ErrUnsafePath)
	}
	return nil
}
func rootIdentity(p string) string {
	i, e := os.Lstat(p)
	if e != nil {
		return ""
	}
	if i.Mode()&os.ModeSymlink != 0 || !i.IsDir() {
		return ""
	}
	if s, ok := i.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("%d:%d:%d", uint64(s.Dev), uint64(s.Ino), uint32(i.Mode().Type()))
	}
	return fmt.Sprintf("%d:%d", i.Size(), i.Mode().Type())
}
func strictDesc(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func noSymlinks(path, root string) error {
	if !strictDesc(path, root) {
		return ErrUnsafePath
	}
	rel, _ := filepath.Rel(root, path)
	cur := root
	for _, p := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, p)
		i, e := os.Lstat(cur)
		if e != nil {
			return e
		}
		if i.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink component", ErrUnsafePath)
		}
	}
	return nil
}

func AllocateWorkspace(c WorkspaceConfig) (*WorkspaceAllocation, error) {
	if c.Store == nil {
		return nil, errors.New("benchmark workspace: ownership store is required")
	}
	if !safeComponent(c.AttemptID) || !safeComponent(c.CaseID) || !safeComponent(c.RunID) {
		return nil, fmt.Errorf("%w: unsafe component", ErrUnsafePath)
	}
	wr, e := canonicalRoot(c.WorkRoot)
	if e != nil {
		return nil, e
	}
	er, e := canonicalRoot(c.EvidenceRoot)
	if e != nil {
		return nil, e
	}
	if wr == er || strictDesc(wr, er) || strictDesc(er, wr) {
		return nil, fmt.Errorf("%w: roots overlap", ErrUnsafePath)
	}
	providedNonce := c.Nonce != ""
	nonce := c.Nonce
	if nonce == "" {
		b := make([]byte, 16)
		if _, e := rand.Read(b); e != nil {
			return nil, e
		}
		nonce = hex.EncodeToString(b)
	}
	c.WorkRoot, c.EvidenceRoot, c.Nonce = wr, er, nonce
	wrFD, err := os.OpenRoot(wr)
	if err != nil {
		return nil, err
	}
	erFD, err := os.OpenRoot(er)
	if err != nil {
		_ = wrFD.Close()
		return nil, err
	}
	closeRoots := true
	defer func() {
		if closeRoots {
			_ = wrFD.Close()
			_ = erFD.Close()
		}
	}()
	wp := filepath.Join(wr, c.RunID, c.CaseID, c.AttemptID)
	ep := filepath.Join(er, c.RunID, c.CaseID, c.AttemptID)
	for _, p := range []string{filepath.Join(wr, c.RunID), filepath.Join(wr, c.RunID, c.CaseID), wp, filepath.Join(er, c.RunID), filepath.Join(er, c.RunID, c.CaseID), ep} {
		if err := mkdirNoSymlink(p); err != nil {
			return nil, err
		}
	}
	if err := noSymlinks(wp, wr); err != nil {
		return nil, err
	}
	if err := noSymlinks(ep, er); err != nil {
		return nil, err
	}
	m := AllocationMarker{AttemptID: c.AttemptID, Nonce: nonce, WorkRoot: wr, EvidenceRoot: er, Workspace: wp, EvidencePath: ep, CreatedAt: time.Now().UTC(), WorkIdentity: rootIdentity(wr), EvidenceIdentity: rootIdentity(er)}
	mp := filepath.Join(wp, ".allocation.json")
	created := false
	mpRel, _ := relToRoot(wr, mp)
	if b, err := wrFD.ReadFile(mpRel); err == nil {
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		// Reuse the durable allocation after a process restart. The marker is
		// authoritative for the nonce and in-flight artifact name.
		if m.AttemptID != c.AttemptID || m.Nonce == "" || (providedNonce && c.Nonce != m.Nonce) || m.WorkRoot != wr || m.EvidenceRoot != er || m.Workspace != wp || m.EvidencePath != ep {
			return nil, ErrUnsafePath
		}
		if m.WorkIdentity != "" && (m.WorkIdentity != rootIdentity(wr) || m.EvidenceIdentity != rootIdentity(er)) {
			return nil, ErrUnsafePath
		}
		c.Nonce = m.Nonce
	} else if errors.Is(err, os.ErrNotExist) {
		f, err := wrFD.OpenFile(mpRel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(m)
		if _, err = f.Write(b); err == nil {
			err = f.Sync()
		}
		f.Close()
		if err != nil {
			return nil, err
		}
		if d, e := wrFD.Open(filepath.Dir(mpRel)); e == nil {
			e = d.Sync()
			_ = d.Close()
			if e != nil {
				return nil, e
			}
		} else {
			return nil, e
		}
		created = true
	} else {
		return nil, err
	}
	workRel, _ := relToRoot(wr, wp)
	evidenceRel, _ := relToRoot(er, ep)
	workFD, err := wrFD.OpenRoot(workRel)
	if err != nil {
		_ = wrFD.Close()
		_ = erFD.Close()
		return nil, err
	}
	evidenceFD, err := erFD.OpenRoot(evidenceRel)
	if err != nil {
		_ = workFD.Close()
		_ = wrFD.Close()
		_ = erFD.Close()
		return nil, err
	}
	o := &WorkspaceAllocation{Config: c, WorkPath: wp, EvidencePath: ep, MarkerPath: mp, Marker: m, lastArtifact: m.ArtifactName, workRootFD: wrFD, evidenceRootFD: erFD, workDirFD: workFD, evidenceDirFD: evidenceFD}
	closeRoots = false
	if created {
		if err := c.Store.SaveOwnership(Ownership{AttemptID: c.AttemptID, Workspace: wp, EvidencePath: ep, Nonce: nonce, WorkRoot: wr, EvidenceRoot: er, CleanupState: "active"}); err != nil {
			_ = o.Close()
			return nil, err
		}
	}
	return o, nil
}

// Close releases the descriptor roots. It is safe to call repeatedly.
func (a *WorkspaceAllocation) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closeLocked()
}
func (a *WorkspaceAllocation) closeLocked() error {
	if a.closed {
		return nil
	}
	a.closed = true
	var err error
	if a.workDirFD != nil {
		err = a.workDirFD.Close()
	}
	if a.evidenceDirFD != nil {
		if e := a.evidenceDirFD.Close(); err == nil {
			err = e
		}
	}
	if a.workRootFD != nil {
		if e := a.workRootFD.Close(); err == nil {
			err = e
		}
	}
	if a.evidenceRootFD != nil {
		if e := a.evidenceRootFD.Close(); err == nil {
			err = e
		}
	}
	return err
}

func (a *WorkspaceAllocation) Capture(src io.Reader, name string) (Artifact, error) {
	return a.CaptureWithOptions(src, name, CaptureOptions{})
}
func (a *WorkspaceAllocation) CaptureWithOptions(src io.Reader, name string, opt CaptureOptions) (Artifact, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !safeComponent(name) {
		return Artifact{}, fmt.Errorf("%w: artifact name", ErrUnsafePath)
	}
	a.lastArtifact = name
	a.Marker.ArtifactName = name
	if err := writeMarkerAtomicRoot(a.workRootFD, mustRel(a.Config.WorkRoot, a.MarkerPath), a.Marker); err != nil {
		return Artifact{}, err
	}
	if err := a.validate(); err != nil {
		return Artifact{}, err
	}
	partialName := "." + a.Config.AttemptID + "." + name + ".partial"
	final := filepath.Join(a.EvidencePath, name)
	if _, e := a.evidenceDirFD.Lstat(name); e == nil {
		own, le := a.Config.Store.LoadOwnership(a.Config.AttemptID)
		if le != nil {
			return Artifact{}, le
		}
		f, oe := a.evidenceDirFD.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if oe != nil {
			return Artifact{}, oe
		}
		h := sha256.New()
		_, ce := io.Copy(h, f)
		st, se := f.Stat()
		f.Close()
		if ce != nil || se != nil {
			return Artifact{}, fmt.Errorf("capture verify: %w", firstErr(ce, se))
		}
		hash := hex.EncodeToString(h.Sum(nil))
		if own.Verified && own.VerifiedHash == hash && own.VerifiedSize == st.Size() && own.VerifiedMarker == markerDigest(a.Marker) {
			return Artifact{Path: final, SHA256: hash, SizeBytes: st.Size(), Verified: true, Metadata: map[string]string{"attempt_id": a.Config.AttemptID}}, nil
		}
		return Artifact{}, errors.New("capture: existing evidence is unverified; explicit reverify required")
	}
	f, err := a.evidenceDirFD.OpenFile(partialName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return Artifact{}, err
	}
	if opt.Failure == FailAfterPartialCreate {
		f.Close()
		_ = a.evidenceDirFD.Remove(partialName)
		return Artifact{}, errors.New("injected capture interruption")
	}
	if _, err = io.Copy(f, src); err == nil {
		if opt.Failure == FailAfterCopy {
			err = errors.New("injected capture interruption")
		}
	}
	if err == nil {
		err = f.Sync()
		if opt.Failure == FailAfterFileSync {
			err = errors.New("injected capture interruption")
		}
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		_ = a.evidenceDirFD.Remove(partialName)
		return Artifact{}, err
	}
	if err = a.evidenceDirFD.Rename(partialName, name); err != nil {
		return Artifact{}, err
	}
	if opt.Failure == FailAfterRename {
		return Artifact{}, errors.New("injected capture interruption")
	}
	fi, err := a.evidenceDirFD.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return Artifact{}, err
	}
	before, err := fi.Stat()
	h := sha256.New()
	if err == nil {
		_, err = io.Copy(h, fi)
	}
	after, e2 := fi.Stat()
	fi.Close()
	if err == nil && e2 == nil && (before.Size() != after.Size() || before.ModTime() != after.ModTime()) {
		err = errors.New("capture: evidence replaced during hash")
	}
	if err != nil {
		return Artifact{}, err
	}
	if opt.Failure == FailAfterHash {
		return Artifact{}, errors.New("injected capture interruption")
	}
	if opt.Failure == FailAfterDirectorySync || opt.Failure == FailBeforeVerifiedCommit {
		return Artifact{}, errors.New("injected capture interruption")
	}
	st := after
	d, err := a.evidenceDirFD.Open(".")
	if err == nil {
		err = d.Sync()
		d.Close()
	}
	if err != nil {
		return Artifact{}, err
	}
	artifact := Artifact{Path: final, SHA256: hex.EncodeToString(h.Sum(nil)), SizeBytes: st.Size(), Verified: true, Metadata: map[string]string{"attempt_id": a.Config.AttemptID}}
	if err := a.Config.Store.MarkVerifiedCAS(context.Background(), a.Config.AttemptID, a.Config.Nonce, markerDigest(a.Marker), artifact.SHA256, artifact.SizeBytes, a.Marker); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}
func (a *WorkspaceAllocation) validate() error {
	for _, root := range []string{a.Config.WorkRoot, a.Config.EvidenceRoot} {
		i, e := os.Lstat(root)
		if e != nil || i.Mode()&os.ModeSymlink != 0 || !i.IsDir() {
			return ErrUnsafePath
		}
	}
	if err := noSymlinks(a.WorkPath, a.Config.WorkRoot); err != nil {
		return err
	}
	if err := noSymlinks(a.EvidencePath, a.Config.EvidenceRoot); err != nil {
		return err
	}
	var m AllocationMarker
	b, e := a.workRootFD.ReadFile(mustRel(a.Config.WorkRoot, a.MarkerPath))
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &m); e != nil {
		return e
	}
	if m.AttemptID != a.Config.AttemptID || m.Nonce == "" || m.Nonce != a.Config.Nonce || m.WorkRoot != a.Config.WorkRoot || m.EvidenceRoot != a.Config.EvidenceRoot || m.Workspace != a.WorkPath || m.EvidencePath != a.EvidencePath {
		return ErrUnsafePath
	}
	if m.WorkIdentity != "" && (m.WorkIdentity != rootIdentity(a.Config.WorkRoot) || m.EvidenceIdentity != rootIdentity(a.Config.EvidenceRoot)) {
		return ErrUnsafePath
	}
	return nil
}

// Reverify performs the only permitted recovery after a post-rename interruption.
func (a *WorkspaceAllocation) Reverify(name string) (Artifact, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !safeComponent(name) {
		return Artifact{}, ErrUnsafePath
	}
	if err := a.validate(); err != nil {
		return Artifact{}, err
	}
	own, err := a.Config.Store.LoadOwnership(a.Config.AttemptID)
	if err != nil || own.Verified || own.Nonce != a.Config.Nonce || own.Workspace != a.WorkPath || own.EvidencePath != a.EvidencePath {
		if err == nil {
			err = errors.New("reverify: ownership is already verified or mismatched")
		}
		return Artifact{}, err
	}
	f, err := a.evidenceDirFD.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return Artifact{}, err
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	st, se := f.Stat()
	f.Close()
	if err != nil {
		return Artifact{}, err
	}
	if se != nil {
		return Artifact{}, se
	}
	art := Artifact{Path: filepath.Join(a.EvidencePath, name), SHA256: hex.EncodeToString(h.Sum(nil)), SizeBytes: st.Size(), Verified: true, Metadata: map[string]string{"attempt_id": a.Config.AttemptID}}
	if err := a.Config.Store.MarkVerifiedCAS(context.Background(), a.Config.AttemptID, a.Config.Nonce, markerDigest(a.Marker), art.SHA256, art.SizeBytes, a.Marker); err != nil {
		return Artifact{}, err
	}
	return art, nil
}
func (a *WorkspaceAllocation) Cleanup(o CleanupOptions) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	fail := func(err error) error {
		_ = a.Config.Store.MarkCleanupState(a.Config.AttemptID, "error", err)
		return err
	}
	if err := a.validate(); err != nil {
		return fail(err)
	}
	own, err := a.Config.Store.LockOwnership(a.Config.AttemptID)
	if err != nil {
		return fail(err)
	}
	if own.Nonce != a.Config.Nonce || own.Workspace != a.WorkPath || own.WorkRoot != a.Config.WorkRoot || own.EvidenceRoot != a.Config.EvidenceRoot {
		return fail(ErrUnsafePath)
	}
	if own.Verified && o.DiscardUnverified {
		return fail(ErrVerifiedImmutable)
	}
	if !own.Verified && !o.DiscardUnverified {
		return fail(errors.New("cleanup requires verified evidence or explicit discard"))
	}
	if !own.Verified && o.DiscardUnverified {
		dir, err := a.evidenceDirFD.Open(".")
		var ents []os.FileInfo
		if err == nil {
			ents, err = dir.Readdir(-1)
			_ = dir.Close()
		}
		if err != nil {
			return fail(err)
		}
		for _, e := range ents {
			// Unverified evidence is disposable only when the filename is
			// attempt-scoped (partial or final names carrying the attempt id).
			if (strings.HasSuffix(e.Name(), ".partial") && strings.HasPrefix(e.Name(), "."+a.Config.AttemptID+".")) || e.Name() == a.lastArtifact {
				if err := a.evidenceDirFD.Remove(e.Name()); err != nil {
					return fail(err)
				}
			}
		}
	}
	// Revalidate immediately before filesystem removal: DB work may have taken time.
	if err := a.validate(); err != nil {
		return fail(err)
	}
	cleanup := o.Cleanup
	if cleanup == nil {
		cleanup = a.Config.Cleanup
	}
	if cleanup == nil {
		err := errors.New("cleanup requires a transaction callback")
		_ = a.Config.Store.MarkCleanupState(a.Config.AttemptID, "files_pending", err)
		return err
	}
	if err := a.Config.Store.CleanupTransaction(context.Background(), a.Config.AttemptID, cleanup); err != nil {
		_ = a.Config.Store.MarkCleanupState(a.Config.AttemptID, "files_pending", err)
		return err
	}
	workRel := mustRel(a.Config.WorkRoot, a.WorkPath)
	if err := a.workRootFD.RemoveAll(workRel); err != nil {
		_ = a.Config.Store.MarkCleanupState(a.Config.AttemptID, "files_pending", err)
		return err
	}
	err = a.Config.Store.MarkCleanupState(a.Config.AttemptID, "cleaned", nil)
	_ = a.closeLocked()
	return err
}
