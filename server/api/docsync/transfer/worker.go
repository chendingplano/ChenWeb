package transfer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Work runs in the detached export process with the lock inherited from
// StartWorker. The lock must stay open until all work and publication end.
func (s *Server) Work(ctx context.Context, runID int64, lock *os.File) (resultErr error) {
	if runID <= 0 || lock == nil {
		return errors.New("invalid document export worker")
	}
	defer lock.Close()
	dir := s.runDir(runID)
	lockInfo, err := lock.Stat()
	if err != nil {
		return err
	}
	workerInfo, err := os.Stat(filepath.Join(dir, workerFile))
	if err != nil || !os.SameFile(lockInfo, workerInfo) {
		return errors.New("invalid document export worker lock")
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("document export worker does not hold the pack lock")
	}
	idx, err := readIndex(dir, runID)
	if err != nil || idx.State != StatePacking || idx.RunID != runID || idx.Name != archiveName(runID, idx.TarNumber) {
		return errors.New("document export pack request is unavailable")
	}
	publicError := "document export packer failed"
	defer func() {
		if resultErr != nil {
			idx.State, idx.Error = StateFailed, publicError
			resultErr = errors.Join(resultErr, writeIndex(dir, idx))
		}
	}()
	f, err := os.Open(filepath.Join(dir, requestFile))
	if err != nil {
		return err
	}
	var request PackRequest
	err = errors.Join(readJSON(f, &request), f.Close())
	if err != nil {
		return err
	}
	if request.RunID != runID || request.TarNumber != idx.TarNumber {
		return errors.New("document export pack request does not match its index")
	}
	if s.Callbacks.Pack == nil {
		return errors.New("document export source is unavailable")
	}
	packing := filepath.Join(dir, packingDir)
	if err := os.RemoveAll(packing); err != nil {
		return err
	}
	if err := removeIfPresent(filepath.Join(dir, idx.Name)); err != nil {
		return err
	}
	if err := checkSpace(dir, request.MaxBytes); err != nil {
		publicError = err.Error()
		return err
	}

	if err := os.Mkdir(packing, 0700); err != nil {
		return err
	}
	defer os.RemoveAll(packing)
	temp := filepath.Join(packing, idx.Name)
	packed, err := s.Callbacks.Pack(ctx, request, temp)
	if err != nil {
		return err
	}
	idx.PackResult = packed
	if err := addChecksums(temp, request.SegmentBytes, &idx); err != nil {
		return err
	}
	if err := os.Rename(temp, filepath.Join(dir, idx.Name)); err != nil {
		return err
	}
	idx.State = StateReady
	if err := writeIndex(dir, idx); err != nil {
		return err
	}
	if err := removeIfPresent(filepath.Join(dir, requestFile)); err != nil && s.Logger != nil {
		s.Logger.Warn("could not remove document export pack request", "run_id", runID, "tar", idx.Name, "error", err)
	}
	s.info("document export archive packed", "run_id", runID, "tar", idx.Name, "bytes", idx.Size)
	return nil
}

func checkSpace(dir string, maxBytes int64) error {
	const margin = 256 << 20
	if maxBytes <= 0 || maxBytes > (math.MaxInt64-margin)/2 {
		return errors.New("invalid document export archive size limit")
	}
	var free unix.Statfs_t
	if err := unix.Statfs(dir, &free); err != nil {
		return errors.New("could not check free disk space for document export")
	}
	if free.Bsize <= 0 || uint64(free.Bavail) <= uint64((2*maxBytes+margin)/int64(free.Bsize)) {
		return errors.New("insufficient free disk space for document export archive, packing workspace and margin")
	}
	return nil
}

func addChecksums(path string, segmentBytes int64, idx *Index) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	const maxSegments = maxRequestSize / 100
	if !info.Mode().IsRegular() || info.Size() <= 0 || segmentBytes <= 0 ||
		1+(info.Size()-1)/segmentBytes > maxSegments {
		return errors.New("invalid document export archive or segment size")
	}
	if err := f.Chmod(0600); err != nil {
		return err
	}
	whole := sha256.New()
	segment := sha256.New()
	writer := io.MultiWriter(whole, segment)
	var hashes []string
	for remaining := info.Size(); remaining > 0; {
		n := min(segmentBytes, remaining)
		segment.Reset()
		if _, err := io.CopyN(writer, f, n); err != nil {
			return fmt.Errorf("hash document export segment: %w", err)
		}
		hashes = append(hashes, hex.EncodeToString(segment.Sum(nil)))
		remaining -= n
	}
	idx.Size = info.Size()
	idx.SHA256 = hex.EncodeToString(whole.Sum(nil))
	idx.SegmentSize = segmentBytes
	idx.SegmentSHA256 = hashes
	return f.Sync()
}
