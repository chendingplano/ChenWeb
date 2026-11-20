package transfer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestForcedCommandAcceptsOnlyExportProtocol(t *testing.T) {
	for _, s := range []string{
		"whoami", "export pack --run 8 --tar ../../x", "export status --run +1", "export status --run 01",
		"export fetch --run 3 --tar 4-0001.tar.gz --segment 0 --offset 0",
		"export fetch --run 3 --tar 3-0001.tar.gz --segment -1 --offset 0",
		"export list; whoami", "export ack --run 1 --tar 1-0001.tar.gz extra", "export logout --token secret",
	} {
		if _, err := parseCommand(s); err == nil {
			t.Errorf("unexpectedly accepted %q", s)
		}
	}
	for _, s := range []string{
		"export login", "export logout", "export list", "export pack --run 3 --tar 1",
		"export status --run 3", "export ack --run 3 --tar 3-0001.tar.gz",
		"export fetch --run 3 --tar 3-0001.tar.gz --segment 0 --offset 0", "export ack --run 3",
	} {
		if _, err := parseCommand(s); err != nil {
			t.Errorf("rejected %q: %v", s, err)
		}
	}
}

func TestDownloadResumesAndRetriesCorruptSegment(t *testing.T) {
	all := "abcdefghijkl"
	digest := func(s string) string {
		b := sha256.Sum256([]byte(s))
		return hex.EncodeToString(b[:])
	}
	index := Index{
		RunID: 3, TarNumber: 1, State: StateReady, Name: "3-0001.tar.gz",
		Size: 12, SegmentSize: 4, SHA256: digest(all),
		SegmentSHA256: []string{digest("abcd"), digest("efgh"), digest("ijkl")},
	}
	name := filepath.Join(t.TempDir(), index.Name)
	if err := os.WriteFile(name+".partial", []byte("abcdef"), 0600); err != nil {
		t.Fatal(err)
	}
	var offsets [][2]int64
	fetch := func(_ context.Context, segment int, offset int64, w io.Writer) error {
		offsets = append(offsets, [2]int64{int64(segment), offset})
		if len(offsets) == 1 {
			_, _ = io.WriteString(w, "Xh")
			return nil
		}
		if len(offsets) == 2 {
			_, _ = io.WriteString(w, "e")
			return errors.New("connection broke")
		}
		_, err := io.WriteString(w, all[segment*4+int(offset):(segment+1)*4])
		return err
	}
	var retries [][2]int64
	onRetry := func(_ context.Context, runID int64, segment int, offset int64, err error) {
		if runID != index.RunID || err == nil {
			t.Errorf("invalid retry notification for run %d: %v", runID, err)
		}
		retries = append(retries, [2]int64{int64(segment), offset})
	}
	if err := download(context.Background(), index, name, time.Nanosecond, time.Nanosecond, nil, onRetry, fetch); err != nil {
		t.Fatal(err)
	}
	if want := [][2]int64{{1, 0}, {1, 1}}; !reflect.DeepEqual(retries, want) {
		t.Fatalf("got retry offsets %v, want %v", retries, want)
	}
	got, err := os.ReadFile(name)
	if err != nil || string(got) != all {
		t.Fatalf("download = %q, %v", got, err)
	}
	want := [][2]int64{{1, 2}, {1, 0}, {1, 1}, {2, 0}}
	if !reflect.DeepEqual(offsets, want) {
		t.Fatalf("got offsets %v, want %v", offsets, want)
	}
}

type testAuth struct {
	active bool
}

func (a *testAuth) LoginNativePassword(_ context.Context, _, _ string) (string, error) {
	return "primary", nil
}

func (a *testAuth) ValidateNativeSession(_ context.Context, token string) (string, error) {
	switch {
	case token == "primary" && a.active:
		return "account-1", nil
	case token == "other":
		return "account-2", nil
	default:
		return "", ErrSession
	}
}

func (a *testAuth) LogoutNativeSession(_ context.Context, _ string) error {
	a.active = false
	return nil
}

func TestExportWorkerOwnershipAndAcknowledgment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	outbox := filepath.Join(t.TempDir(), "outbox")
	if err := os.Mkdir(outbox, 0700); err != nil {
		t.Fatal(err)
	}
	if err := checkSpace(outbox, 20); err != nil {
		t.Skip(err)
	}
	identity := &testAuth{active: true}
	s := Server{Auth: identity, Outbox: outbox}
	release := make(chan struct{})
	finished := make(chan error, 1)
	s.Callbacks.Pack = func(ctx context.Context, req PackRequest, path string) (PackResult, error) {
		select {
		case <-ctx.Done():
			return PackResult{}, ctx.Err()
		case <-release:
			return PackResult{Included: req.Work}, os.WriteFile(path, []byte("sync content"), 0600)
		}
	}
	starts := 0
	s.StartWorker = func(runID int64, lock *os.File) error {
		starts++
		fd, err := unix.Dup(int(lock.Fd()))
		if err != nil {
			return err
		}
		go func() { finished <- s.Work(ctx, runID, os.NewFile(uintptr(fd), "worker lock")) }()
		return nil
	}
	call := func(command, token string, request any) (string, error) {
		var body json.RawMessage
		if request != nil {
			raw, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			body = raw
		}
		raw, err := json.Marshal(envelope{Token: token, Body: body})
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err = s.Serve(ctx, command, bytes.NewReader(raw), &out)
		return out.String(), err
	}
	status := func() Index {
		t.Helper()
		reply, err := call("export status --run 7", "primary", nil)
		if err != nil {
			t.Fatal(err)
		}
		var idx Index
		if err := json.Unmarshal([]byte(reply), &idx); err != nil {
			t.Fatal(err)
		}
		return idx
	}

	request := PackRequest{RunID: 7, TarNumber: 1, MaxBytes: 20, SegmentBytes: 4, Work: json.RawMessage(`[1]`)}
	for range 2 {
		if _, err := call("export pack --run 7 --tar 1", "primary", request); err != nil {
			t.Fatal(err)
		}
	}
	if got := status().State; got != StatePacking || starts != 1 {
		t.Fatalf("status = %s, worker starts = %d", got, starts)
	}
	if _, err := call("export ack --run 7", "primary", nil); err == nil {
		t.Fatal("ack succeeded while worker was active")
	}
	if _, err := call("export status --run 7", "other", nil); err == nil {
		t.Fatal("other account read run status")
	}
	close(release)
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	}

	idx := status()
	if err := idx.validateReady(); err != nil || string(idx.Included) != "[1]" {
		t.Fatalf("ready index = %+v, error = %v", idx, err)
	}
	if _, err := call("export pack --run 7 --tar 1", "primary", request); err != nil || starts != 1 {
		t.Fatalf("ready archive repack: err = %v, starts = %d", err, starts)
	}
	fetch := "export fetch --run 7 --tar 7-0001.tar.gz --segment 1 --offset 2"
	if out, err := call(fetch, "primary", nil); out != "on" || err != nil {
		t.Fatalf("segment suffix = %q, error = %v", out, err)
	}
	identity.active = false
	if out, err := call(fetch, "primary", nil); out != "" || !errors.Is(err, ErrSession) {
		t.Fatalf("revoked session response = %q, error = %v", out, err)
	}
	if err := s.Serve(ctx, "export login", strings.NewReader(`{"phone":"5550100123","password":"test"}`), io.Discard); !errors.Is(err, ErrSession) {
		t.Fatalf("revoked account login error = %v", err)
	}
	identity.active = true
	for range 2 {
		if _, err := call("export ack --run 7 --tar 7-0001.tar.gz", "primary", nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := status().State; got != StateMissing {
		t.Fatalf("status after ack = %s", got)
	}
	if _, err := call("export ack --run 7", "primary", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outbox, "7")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("run outbox survived ack: %v", err)
	}
}
