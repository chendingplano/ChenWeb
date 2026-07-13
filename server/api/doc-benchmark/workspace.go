package docbenchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
}
type AllocationMarker struct {
	AttemptID        string    `json:"attempt_id"`
	Nonce            string    `json:"nonce"`
	WorkRoot         string    `json:"work_root"`
	EvidenceRoot     string    `json:"evidence_root"`
	Workspace        string    `json:"workspace"`
	CreatedAt        time.Time `json:"created_at"`
	WorkIdentity     string    `json:"work_identity"`
	EvidenceIdentity string    `json:"evidence_identity"`
}
type WorkspaceAllocation struct {
	Config                             WorkspaceConfig
	WorkPath, EvidencePath, MarkerPath string
	Marker                             AllocationMarker
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
type CleanupOptions struct{ DiscardUnverified bool }
type Ownership struct {
	AttemptID, Workspace, EvidencePath, Nonce, WorkRoot, EvidenceRoot string
	Verified                                                          bool
	CleanupState                                                      string
}
type OwnershipStore interface {
	LoadOwnership(attemptID string) (Ownership, error)
	SaveOwnership(Ownership) error
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
	if err := os.MkdirAll(abs, 0700); err != nil {
		return "", err
	}
	li, err := os.Lstat(abs)
	if err != nil {
		return "", err
	}
	if li.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%w: symlink root", ErrUnsafePath)
	}
	return filepath.Clean(abs), nil
}
func rootIdentity(p string) string {
	i, e := os.Stat(p)
	if e != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d:%d", i.ModTime().UnixNano(), i.Size(), i.Mode())
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
	nonce := c.Nonce
	if nonce == "" {
		nonce = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	c.WorkRoot, c.EvidenceRoot, c.Nonce = wr, er, nonce
	wp := filepath.Join(wr, c.RunID, c.CaseID, c.AttemptID)
	ep := filepath.Join(er, c.RunID, c.CaseID, c.AttemptID)
	for _, p := range []string{filepath.Join(wr, c.RunID), filepath.Join(wr, c.RunID, c.CaseID), wp, filepath.Join(er, c.RunID), filepath.Join(er, c.RunID, c.CaseID), ep} {
		if err := os.MkdirAll(p, 0700); err != nil {
			return nil, err
		}
	}
	if err := noSymlinks(wp, wr); err != nil {
		return nil, err
	}
	if err := noSymlinks(ep, er); err != nil {
		return nil, err
	}
	m := AllocationMarker{AttemptID: c.AttemptID, Nonce: nonce, WorkRoot: wr, EvidenceRoot: er, Workspace: wp, CreatedAt: time.Now().UTC(), WorkIdentity: rootIdentity(wr), EvidenceIdentity: rootIdentity(er)}
	mp := filepath.Join(wp, ".allocation.json")
	b, _ := json.Marshal(m)
	f, err := os.OpenFile(mp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	f.Close()
	if err != nil {
		return nil, err
	}
	o := &WorkspaceAllocation{Config: c, WorkPath: wp, EvidencePath: ep, MarkerPath: mp, Marker: m}
	if c.Store != nil {
		if err := c.Store.SaveOwnership(Ownership{AttemptID: c.AttemptID, Workspace: wp, Nonce: nonce, WorkRoot: wr, EvidenceRoot: er, CleanupState: "active"}); err != nil {
			return nil, err
		}
	}
	return o, nil
}

func (a *WorkspaceAllocation) Capture(src io.Reader, name string) (Artifact, error) {
	return a.CaptureWithOptions(src, name, CaptureOptions{})
}
func (a *WorkspaceAllocation) CaptureWithOptions(src io.Reader, name string, opt CaptureOptions) (Artifact, error) {
	if !safeComponent(name) {
		return Artifact{}, fmt.Errorf("%w: artifact name", ErrUnsafePath)
	}
	if err := a.validate(); err != nil {
		return Artifact{}, err
	}
	partial := filepath.Join(a.EvidencePath, "."+a.Config.AttemptID+"."+name+".partial")
	final := filepath.Join(a.EvidencePath, name)
	if _, e := os.Lstat(final); e == nil {
		return Artifact{}, ErrVerifiedImmutable
	}
	f, err := os.OpenFile(partial, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return Artifact{}, err
	}
	if opt.Failure == FailAfterPartialCreate {
		f.Close()
		os.Remove(partial)
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
		os.Remove(partial)
		return Artifact{}, err
	}
	if err = os.Rename(partial, final); err != nil {
		return Artifact{}, err
	}
	if opt.Failure == FailAfterRename {
		return Artifact{}, errors.New("injected capture interruption")
	}
	fi, err := os.Open(final)
	if err != nil {
		return Artifact{}, err
	}
	h := sha256.New()
	_, err = io.Copy(h, fi)
	fi.Close()
	if err != nil {
		return Artifact{}, err
	}
	if opt.Failure == FailAfterHash {
		return Artifact{}, errors.New("injected capture interruption")
	}
	if opt.Failure == FailAfterDirectorySync || opt.Failure == FailBeforeVerifiedCommit {
		return Artifact{}, errors.New("injected capture interruption")
	}
	st, err := os.Stat(final)
	if err != nil {
		return Artifact{}, err
	}
	d, err := os.Open(a.EvidencePath)
	if err == nil {
		err = d.Sync()
		d.Close()
	}
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{Path: final, SHA256: hex.EncodeToString(h.Sum(nil)), SizeBytes: st.Size(), Verified: true, Metadata: map[string]string{"attempt_id": a.Config.AttemptID}}, nil
}
func (a *WorkspaceAllocation) validate() error {
	if err := noSymlinks(a.WorkPath, a.Config.WorkRoot); err != nil {
		return err
	}
	if err := noSymlinks(a.EvidencePath, a.Config.EvidenceRoot); err != nil {
		return err
	}
	var m AllocationMarker
	b, e := os.ReadFile(a.MarkerPath)
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &m); e != nil {
		return e
	}
	if m.AttemptID != a.Config.AttemptID || m.Nonce != a.Config.Nonce || m.WorkRoot != a.Config.WorkRoot || m.EvidenceRoot != a.Config.EvidenceRoot {
		return ErrUnsafePath
	}
	if m.WorkIdentity != "" && (m.WorkIdentity != rootIdentity(a.Config.WorkRoot) || m.EvidenceIdentity != rootIdentity(a.Config.EvidenceRoot)) {
		return ErrUnsafePath
	}
	return nil
}
func (a *WorkspaceAllocation) Cleanup(o CleanupOptions) error {
	if err := a.validate(); err != nil {
		return err
	}
	if a.Config.Store != nil {
		own, err := a.Config.Store.LoadOwnership(a.Config.AttemptID)
		if err != nil {
			return err
		}
		if own.Nonce != a.Config.Nonce || own.Workspace != a.WorkPath || own.WorkRoot != a.Config.WorkRoot || own.EvidenceRoot != a.Config.EvidenceRoot {
			return ErrUnsafePath
		}
		if own.Verified && !o.DiscardUnverified {
			return ErrVerifiedImmutable
		}
	}
	if o.DiscardUnverified {
		ents, _ := os.ReadDir(a.EvidencePath)
		for _, e := range ents {
			if strings.HasSuffix(e.Name(), ".partial") {
				os.Remove(filepath.Join(a.EvidencePath, e.Name()))
			}
		}
	}
	return nil
}
