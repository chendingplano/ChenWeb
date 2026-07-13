package docbenchmark

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testOwnershipStore struct {
	own                                    Ownership
	lockErr, adapterErr, inputErr, markErr error
	states                                 []string
	callbackCalled                         bool
}

func (s *testOwnershipStore) SaveOwnership(o Ownership) error         { s.own = o; return nil }
func (s *testOwnershipStore) LoadOwnership(string) (Ownership, error) { return s.own, nil }
func (s *testOwnershipStore) LockOwnership(string) (Ownership, error) {
	if s.lockErr != nil {
		return Ownership{}, s.lockErr
	}
	return s.own, nil
}
func (s *testOwnershipStore) MarkVerified(_ string, hash string, size int64, m AllocationMarker) error {
	if s.markErr != nil {
		return s.markErr
	}
	s.own.Verified = true
	s.own.VerifiedHash = hash
	s.own.VerifiedSize = size
	s.own.VerifiedMarker = markerDigest(m)
	return nil
}
func (s *testOwnershipStore) MarkVerifiedCAS(_ context.Context, _ string, nonce, markerHash, hash string, size int64, m AllocationMarker) error {
	if s.own.Verified || s.own.Nonce != "" && s.own.Nonce != nonce || s.own.VerifiedMarker != "" && s.own.VerifiedMarker != markerHash {
		return errors.New("capture: ownership CAS failed")
	}
	return s.MarkVerified("", hash, size, m)
}
func (s *testOwnershipStore) MarkCleanupState(_ string, state string, cause error) error {
	s.states = append(s.states, state)
	if cause != nil {
		return cause
	}
	s.own.CleanupState = state
	return nil
}

type testCleanupTx struct{ store *testOwnershipStore }

func (t testCleanupTx) DeleteProductionRows() error { return nil }
func (t testCleanupTx) DeleteInput() error          { return nil }
func (t testCleanupTx) MarkState(state string, _ error) error {
	t.store.states = append(t.store.states, state)
	return nil
}

func (s *testOwnershipStore) CleanupTransaction(_ context.Context, _ string, fn func(CleanupTx) error) error {
	s.callbackCalled = true
	if s.adapterErr != nil {
		return s.adapterErr
	}
	return fn(testCleanupTx{store: s})
}
func testConfig(d string, st *testOwnershipStore) WorkspaceConfig {
	return WorkspaceConfig{WorkRoot: filepath.Join(d, "w"), EvidenceRoot: filepath.Join(d, "e"), AttemptID: "a", CaseID: "c", RunID: "r", Store: st}
}

func TestAllocateWorkspaceRejectsTraversalAndOverlap(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "evidence")
	work := filepath.Join(root, "work")
	if _, err := AllocateWorkspace(WorkspaceConfig{WorkRoot: work, EvidenceRoot: evidence, AttemptID: "a-1", CaseID: "case", RunID: "run", Store: &testOwnershipStore{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := AllocateWorkspace(WorkspaceConfig{WorkRoot: root, EvidenceRoot: filepath.Join(root, "e"), AttemptID: "../bad", CaseID: "c", RunID: "r", Store: &testOwnershipStore{}}); err == nil || !strings.Contains(err.Error(), "component") {
		t.Fatalf("expected unsafe id, got %v", err)
	}
	if _, err := AllocateWorkspace(WorkspaceConfig{WorkRoot: root, EvidenceRoot: filepath.Join(root, "sub"), AttemptID: "a", CaseID: "c", RunID: "r", Store: &testOwnershipStore{}}); err == nil {
		t.Fatal("expected overlapping roots rejection")
	}
}

func TestCaptureDurableAndCleanupNeverDeletesVerified(t *testing.T) {
	d := t.TempDir()
	st := &testOwnershipStore{}
	a, err := AllocateWorkspace(testConfig(d, st))
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Capture(strings.NewReader("hello"), "actual.json")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Verified || got.SizeBytes != 5 || got.SHA256 == "" {
		t.Fatalf("bad artifact: %+v", got)
	}
	if err := a.Cleanup(CleanupOptions{Cleanup: func(tx CleanupTx) error { return tx.DeleteProductionRows() }}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(got.Path); err != nil {
		t.Fatalf("verified evidence removed: %v", err)
	}
}

func TestCaptureFailurePointsLeaveRetryableEvidence(t *testing.T) {
	points := []CaptureFailurePoint{FailAfterPartialCreate, FailAfterCopy, FailAfterFileSync, FailAfterRename, FailAfterHash, FailAfterDirectorySync, FailBeforeVerifiedCommit}
	for _, p := range points {
		t.Run(string(p), func(t *testing.T) {
			d := t.TempDir()
			a, e := AllocateWorkspace(testConfig(d, &testOwnershipStore{}))
			if e != nil {
				t.Fatal(e)
			}
			_, _ = a.CaptureWithOptions(strings.NewReader("x"), "f", CaptureOptions{Failure: p})
			if p == FailAfterRename || p == FailAfterHash || p == FailAfterDirectorySync || p == FailBeforeVerifiedCommit {
				if _, e = a.Reverify("f"); e != nil {
					t.Fatal(e)
				}
			}
			if _, e = a.Capture(strings.NewReader("x"), "f"); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestRestartDiscardUnverifiedRemovesPersistedArtifactAndPartial(t *testing.T) {
	d := t.TempDir()
	st := &testOwnershipStore{}
	cfg := testConfig(d, st)
	a, err := AllocateWorkspace(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.CaptureWithOptions(strings.NewReader("payload"), "result.json", CaptureOptions{Failure: FailAfterRename}); err == nil {
		t.Fatal("expected interrupted capture")
	}
	partial := filepath.Join(a.EvidencePath, "."+cfg.AttemptID+".result.json.partial")
	if err = os.WriteFile(partial, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(a.EvidencePath, "other.json")
	if err = os.WriteFile(other, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}

	// A fresh allocation object models a process restart; it must recover the
	// marker's artifact name so discard can remove both exact paths.
	a2, err := AllocateWorkspace(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if a2.Marker.ArtifactName != "result.json" || a2.lastArtifact != "result.json" {
		t.Fatalf("persisted artifact not restored: marker=%q last=%q", a2.Marker.ArtifactName, a2.lastArtifact)
	}
	if err = a2.Cleanup(CleanupOptions{DiscardUnverified: true, Cleanup: func(CleanupTx) error { return nil }}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(a2.EvidencePath, "result.json")); !os.IsNotExist(err) {
		t.Fatalf("final evidence remains: %v", err)
	}
	if _, err = os.Stat(partial); !os.IsNotExist(err) {
		t.Fatalf("partial evidence remains: %v", err)
	}
	if _, err = os.Stat(other); err != nil {
		t.Fatalf("unrelated evidence removed: %v", err)
	}
}

func TestCleanupPersistsPendingAndRejectsVerifiedDiscard(t *testing.T) {
	d := t.TempDir()
	st := &testOwnershipStore{}
	a, err := AllocateWorkspace(testConfig(d, st))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Capture(strings.NewReader("ok"), "f"); err != nil {
		t.Fatal(err)
	}
	if err = a.Cleanup(CleanupOptions{DiscardUnverified: true, Cleanup: func(tx CleanupTx) error { return nil }}); !errors.Is(err, ErrVerifiedImmutable) {
		t.Fatalf("discard verified: %v", err)
	}
	st2 := &testOwnershipStore{adapterErr: errors.New("db down")}
	a2, err := AllocateWorkspace(testConfig(filepath.Join(d, "x"), st2))
	if err != nil {
		t.Fatal(err)
	}
	if err = a2.Cleanup(CleanupOptions{DiscardUnverified: true, Cleanup: func(tx CleanupTx) error { return nil }}); err == nil || len(st2.states) == 0 || st2.states[0] != "files_pending" {
		t.Fatalf("pending state: err=%v states=%v", err, st2.states)
	}
}

func TestCleanupTransactionCallbackErrorDoesNotCommit(t *testing.T) {
	s := &testOwnershipStore{}
	called := false
	err := s.CleanupTransaction(context.Background(), "a", func(CleanupTx) error { called = true; return errors.New("adapter failed") })
	if err == nil || !called {
		t.Fatalf("expected callback error, called=%v err=%v", called, err)
	}
	if s.own.Verified || len(s.states) != 0 {
		t.Fatalf("transaction state changed: %+v", s)
	}
}

func TestWorkspaceCleanupInvokesCallbackAndRollsBackFilesystem(t *testing.T) {
	d := t.TempDir()
	st := &testOwnershipStore{}
	a, err := AllocateWorkspace(testConfig(d, st))
	if err != nil {
		t.Fatal(err)
	}
	art, err := a.Capture(strings.NewReader("payload"), "result.json")
	if err != nil {
		t.Fatal(err)
	}
	called := false
	wantErr := errors.New("adapter cleanup failed")
	err = a.Cleanup(CleanupOptions{Cleanup: func(CleanupTx) error {
		called = true
		return wantErr
	}})
	if !errors.Is(err, wantErr) || !called {
		t.Fatalf("callback error: called=%v err=%v", called, err)
	}
	if _, err := os.Stat(a.WorkPath); err != nil {
		t.Fatalf("worktree removed after rollback: %v", err)
	}
	if _, err := os.Stat(art.Path); err != nil {
		t.Fatalf("evidence removed after rollback: %v", err)
	}
	if len(st.states) == 0 || st.states[len(st.states)-1] != "files_pending" {
		t.Fatalf("cleanup state=%v", st.states)
	}
}

func TestCaptureRejectsMarkerMismatchAndRootReplacement(t *testing.T) {
	d := t.TempDir()
	st := &testOwnershipStore{}
	a, err := AllocateWorkspace(testConfig(d, st))
	if err != nil {
		t.Fatal(err)
	}
	st.own.Verified = true
	st.own.VerifiedHash = "bad"
	st.own.VerifiedSize = 1
	st.own.VerifiedMarker = markerDigest(a.Marker)
	if _, err = a.Capture(strings.NewReader("x"), "f"); err != nil { /* initial capture is allowed */
	}
	if err = os.RemoveAll(a.Config.EvidenceRoot); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(d, a.Config.EvidenceRoot); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Capture(strings.NewReader("x"), "g"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("root replacement: %v", err)
	}
}
