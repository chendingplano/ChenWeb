package transfer

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"golang.org/x/sys/unix"
)

const (
	indexFile   = "index.json"
	accountFile = "account"
	requestFile = "request.json"
	workerFile  = ".worker.lock"
	packingDir  = ".packing"
)

type Authenticator interface {
	LoginNativePassword(ctx context.Context, identifier, password string) (string, error)
	ValidateNativeSession(ctx context.Context, token string) (string, error)
	LogoutNativeSession(ctx context.Context, token string) error
}

type Server struct {
	Auth      Authenticator
	Outbox    string
	OutboxTTL time.Duration
	Logger    ApiTypes.JimoLogger
	Callbacks Callbacks
	// StartWorker starts the local pack subprocess. The child inherits the lock
	// file and keeps it open until Work finishes.
	StartWorker func(runID int64, lock *os.File) error
	// RejectedSession reports whether the authentication service refused access.
	RejectedSession func(error) bool
}

// Serve handles one OpenSSH forced command and never executes the original
// command string. The SSH caller sends credentials or a session only on stdin.
func (s *Server) Serve(ctx context.Context, original string, stdin io.Reader, stdout io.Writer) error {
	c, err := parseCommand(original)
	if err != nil {
		return err
	}
	if s.Auth == nil {
		return errors.New("document export authentication is unavailable")
	}
	if c.Action == "login" {
		return s.login(ctx, stdin, stdout)
	}
	var req envelope
	if err := readJSON(stdin, &req); err != nil {
		return err
	}
	if req.Token == "" {
		return ErrSession
	}
	account, err := s.Auth.ValidateNativeSession(ctx, req.Token)
	if err != nil {
		return s.authError(err)
	}
	if account == "" {
		return ErrSession
	}
	s.info("authorized document export command", "account", account, "action", c.Action,
		"run_id", c.RunID, "tar", c.Name, "segment", c.Segment)
	switch c.Action {
	case "logout":
		return s.authError(s.Auth.LogoutNativeSession(ctx, req.Token))
	case "list":
		if s.Callbacks.List == nil {
			return errors.New("document export listing is unavailable")
		}
		z := gzip.NewWriter(stdout)
		err := s.Callbacks.List(ctx, req.Body, z)
		return errors.Join(err, z.Close())
	case "pack":
		var pack PackRequest
		if err := json.Unmarshal(req.Body, &pack); err != nil || pack.RunID != c.RunID ||
			pack.TarNumber != c.TarNumber || pack.MaxBytes <= 0 || pack.SegmentBytes <= 0 ||
			len(pack.Work) == 0 {
			return errors.New("invalid document export pack request")
		}
		return s.pack(c, account, pack)
	case "status", "fetch", "ack":
		return s.readRun(c, account, stdout)
	}
	return errCommand
}

func (s *Server) login(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	var login loginRequest
	if err := readJSON(stdin, &login); err != nil {
		return err
	}
	token, err := s.Auth.LoginNativePassword(ctx, login.Phone, login.Password)
	if err != nil {
		return s.authError(err)
	}
	account, err := s.Auth.ValidateNativeSession(ctx, token)
	if err != nil || account == "" {
		_ = s.Auth.LogoutNativeSession(ctx, token)
		if err != nil {
			return s.authError(err)
		}
		return ErrSession
	}
	s.info("document export account signed in", "account", account)
	if root, err := s.lockOutbox(); err == nil {
		s.prune()
		_ = root.Close()
	} else if s.Logger != nil {
		s.Logger.Warn("could not check expired document sync archives", "error", err)
	}
	if err := json.NewEncoder(stdout).Encode(loginReply{Token: token}); err != nil {
		_ = s.Auth.LogoutNativeSession(ctx, token)
		return err
	}
	return nil
}

func (s *Server) authError(err error) error {
	if err != nil && s.RejectedSession != nil && s.RejectedSession(err) {
		return ErrSession
	}
	return err
}

func (s *Server) info(message string, fields ...any) {
	if s.Logger != nil {
		s.Logger.Info(message, fields...)
	}
}

func (s *Server) runDir(run int64) string {
	return filepath.Join(s.Outbox, strconv.FormatInt(run, 10))
}

func (s *Server) lockOutbox() (*os.File, error) {
	if s.Outbox == "" {
		return nil, errors.New("document export outbox is unavailable")
	}
	if err := os.MkdirAll(s.Outbox, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(s.Outbox)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("document export outbox must be a private directory")
	}
	return lockFile(filepath.Join(s.Outbox, ".lock"), unix.LOCK_EX)
}

func lockFile(filename string, mode int) (*os.File, error) {
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), mode); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func (s *Server) prune() {
	ttl := s.OutboxTTL
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	entries, err := os.ReadDir(s.Outbox)
	if err != nil {
		return
	}
	for _, e := range entries {
		run, ok := decimal(e.Name(), 1)
		if !e.IsDir() || !ok {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) <= ttl {
			continue
		}
		dir := s.runDir(run)
		lock, err := lockFile(filepath.Join(dir, workerFile), unix.LOCK_EX|unix.LOCK_NB)
		if err != nil {
			continue
		}
		if err := os.RemoveAll(dir); err == nil {
			s.info("removed expired document sync outbox", "run_id", run)
		} else if s.Logger != nil {
			s.Logger.Warn("could not remove expired document sync outbox", "run_id", run, "error", err)
		}
		_ = lock.Close()
	}
}

func (s *Server) pack(c command, account string, pack PackRequest) error {
	root, err := s.lockOutbox()
	if err != nil {
		return err
	}
	defer root.Close()
	dir := s.runDir(c.RunID)
	if err := os.Mkdir(dir, 0700); err == nil {
		if err := atomicWrite(dir, accountFile, []byte(account)); err != nil {
			_ = os.RemoveAll(dir)
			return err
		}
		if err := syncDir(s.Outbox); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := ownsRun(dir, account); err != nil {
		return err
	}
	idx, err := readIndex(dir, c.RunID)
	if err != nil {
		return err
	}
	if idx.State != StateMissing && idx.Name != c.Name {
		return fmt.Errorf("document export tar %s must be acknowledged before another is started", idx.Name)
	}
	if idx.State == StateReady {
		return nil
	}
	worker, err := lockFile(filepath.Join(dir, workerFile), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return nil
	}
	if err != nil {
		return err
	}
	defer worker.Close()
	if s.StartWorker == nil {
		return errors.New("document export packer is unavailable")
	}
	request, err := json.Marshal(pack)
	if err != nil {
		return err
	}
	if err := atomicWrite(dir, requestFile, request); err != nil {
		return err
	}
	idx = Index{RunID: c.RunID, TarNumber: c.TarNumber, Name: c.Name, State: StatePacking}
	if err := writeIndex(dir, idx); err != nil {
		return err
	}
	if err := s.StartWorker(c.RunID, worker); err != nil {
		idx.State = StateFailed
		idx.Error = "could not launch document export packer"
		return errors.Join(err, writeIndex(dir, idx))
	}
	s.info("document export pack started", "run_id", c.RunID, "tar", c.Name, "account", account)
	return nil
}

func (s *Server) readRun(c command, account string, out io.Writer) error {
	root, err := s.lockOutbox()
	if err != nil {
		return err
	}
	defer root.Close()
	dir := s.runDir(c.RunID)
	if err := ownsRun(dir, account); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	} else if err != nil {
		switch c.Action {
		case "status":
			return json.NewEncoder(out).Encode(Index{RunID: c.RunID, State: StateMissing})
		case "ack":
			return nil
		default:
			return errors.New("document export run is missing")
		}
	}
	if err := os.Chtimes(dir, time.Now(), time.Now()); err != nil {
		return err
	}
	idx, err := readIndex(dir, c.RunID)
	if err != nil {
		return err
	}
	switch c.Action {
	case "status":
		if idx.State == StatePacking {
			probe, err := lockFile(filepath.Join(dir, workerFile), unix.LOCK_EX|unix.LOCK_NB)
			if err == nil {
				_ = probe.Close()
				// Read again after the lock is acquired, because the worker may
				// have published its final status since the earlier read.
				idx, err = readIndex(dir, c.RunID)
				if err != nil {
					return err
				}
				if idx.State == StatePacking {
					idx.State, idx.Error = StateFailed, "document export packer was interrupted"
				}
			} else if !errors.Is(err, unix.EWOULDBLOCK) {
				return err
			}
		}
		return json.NewEncoder(out).Encode(idx)
	case "fetch":
		if err := idx.validateReady(); err != nil || idx.Name != c.Name || c.Segment >= len(idx.SegmentSHA256) {
			return errors.New("document export segment is unavailable")
		}
		start := int64(c.Segment) * idx.SegmentSize
		count := min(idx.SegmentSize, idx.Size-start)
		if c.Offset > count {
			return errors.New("document export segment offset is out of range")
		}
		in, err := os.Open(filepath.Join(dir, idx.Name))
		if err != nil {
			return err
		}
		defer in.Close()
		// Open the verified archive before allowing ack or pruning to remove it.
		if err := root.Close(); err != nil {
			return err
		}
		n, err := io.CopyN(out, io.NewSectionReader(in, start+c.Offset, count-c.Offset), count-c.Offset)
		s.info("document export segment sent", "run_id", c.RunID, "tar", c.Name,
			"segment", c.Segment, "offset", c.Offset, "bytes", n)
		return err
	case "ack":
		worker, err := lockFile(filepath.Join(dir, workerFile), unix.LOCK_EX|unix.LOCK_NB)
		if errors.Is(err, unix.EWOULDBLOCK) {
			return errors.New("document export packer is active")
		}
		if err != nil {
			return err
		}
		defer worker.Close()
		if c.Name == "" {
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
			if err := syncDir(s.Outbox); err != nil {
				return err
			}
		} else {
			if idx.Name == c.Name {
				if err := writeIndex(dir, Index{RunID: c.RunID, State: StateMissing}); err != nil {
					return err
				}
			}
			if err := removeIfPresent(filepath.Join(dir, c.Name)); err != nil {
				return err
			}
			if err := syncDir(dir); err != nil {
				return err
			}
		}
		s.info("document export acknowledged", "run_id", c.RunID, "tar", c.Name, "account", account)
		return nil
	}
	return errCommand
}

func ownsRun(dir, account string) error {
	raw, err := os.ReadFile(filepath.Join(dir, accountFile))
	if err != nil {
		return err
	}
	if string(raw) != account {
		return errors.New("document export belongs to another account")
	}
	return nil
}

func removeIfPresent(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func readIndex(dir string, run int64) (Index, error) {
	f, err := os.Open(filepath.Join(dir, indexFile))
	if errors.Is(err, os.ErrNotExist) {
		return Index{RunID: run, State: StateMissing}, nil
	}
	if err != nil {
		return Index{}, err
	}
	defer f.Close()
	var idx Index
	err = readJSON(f, &idx)
	return idx, err
}

func writeIndex(dir string, idx Index) error {
	raw, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	return atomicWrite(dir, indexFile, raw)
}

func atomicWrite(dir, name string, raw []byte) error {
	f, err := os.CreateTemp(dir, ".write-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(raw)
	if err := errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), filepath.Join(dir, name)); err != nil {
		return err
	}
	return syncDir(dir)
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
