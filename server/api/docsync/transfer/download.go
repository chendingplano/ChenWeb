package transfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type fetchSegment func(context.Context, int, int64, io.Writer) error

// Download assembles the archive in place and verifies every segment before
// publishing the finished file. Keep the same path on a resumed run.
func (c *Client) Download(ctx context.Context, index Index, path string) error {
	fetch := func(ctx context.Context, segment int, offset int64, w io.Writer) error {
		command := fmt.Sprintf("export fetch --run %d --tar %s --segment %d --offset %d",
			index.RunID, index.Name, segment, offset)
		return c.call(ctx, command, nil, w)
	}
	return download(ctx, index, path, 30*time.Second, 10*time.Minute, c.logger, c.onRetry, fetch)
}

func download(ctx context.Context, index Index, path string, firstRetry, maxRetry time.Duration, logger ApiTypes.JimoLogger, onRetry func(context.Context, int64, int, int64, error), fetch fetchSegment) error {
	if err := index.validateReady(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	if existing, err := os.Open(path); err == nil {
		digest, checkErr := hashHex(existing)
		closeErr := existing.Close()
		if err := errors.Join(checkErr, closeErr); err != nil {
			return err
		}
		if digest == index.SHA256 {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	part := path + ".partial"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	retry := firstRetry
	wait := func(segment int, received int64, reason error) error {
		if logger != nil {
			logger.Warn("retrying document sync transfer", "run_id", index.RunID, "tar", index.Name,
				"segment", segment, "bytes", received, "delay", retry.String(), "error", reason)
		}
		if onRetry != nil {
			onRetry(ctx, index.RunID, segment, received, reason)
		}
		timer := time.NewTimer(retry)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			retry = min(retry*2, maxRetry)
			return nil
		}
	}

	for {
		info, err := f.Stat()
		if err != nil {
			return err
		}
		size := info.Size()
		if size > index.Size {
			if err := f.Truncate(0); err != nil {
				return err
			}
			size = 0
		}

		for segment, want := range index.SegmentSHA256 {
			start := int64(segment) * index.SegmentSize
			end := start + min(index.SegmentSize, index.Size-start)
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				if size >= end {
					got, err := hashHex(io.NewSectionReader(f, start, end-start))
					if err != nil {
						return err
					}
					if got == want {
						retry = firstRetry
						break
					}
					if err := f.Truncate(start); err != nil {
						return err
					}
					size = start
					if err := wait(segment, 0, errors.New("segment SHA-256 mismatch")); err != nil {
						return err
					}
				}

				offset := size - start
				if _, err := f.Seek(size, io.SeekStart); err != nil {
					return err
				}
				w := &segmentWriter{file: f, remaining: end - size}
				fetchErr := fetch(ctx, segment, offset, w)
				if w.writeErr != nil {
					return w.writeErr
				}
				size = end - w.remaining
				if err := f.Sync(); err != nil {
					return err
				}
				if errors.Is(fetchErr, ErrSession) {
					return fetchErr
				}
				if w.overflow {
					if err := f.Truncate(start); err != nil {
						return err
					}
					size = start
					fetchErr = errors.New("unexpected extra segment bytes")
				}
				if size < end {
					if fetchErr == nil {
						fetchErr = io.ErrUnexpectedEOF
					}
					if err := wait(segment, size-start, fetchErr); err != nil {
						return err
					}
				}
			}
		}
		got, err := hashHex(io.NewSectionReader(f, 0, index.Size))
		if err != nil {
			return err
		}
		if got == index.SHA256 {
			if err := f.Sync(); err != nil {
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
			if err := os.Rename(part, path); err != nil {
				return err
			}
			if err := syncDir(filepath.Dir(path)); err != nil {
				return err
			}
			if logger != nil {
				logger.Info("document sync archive fetched", "run_id", index.RunID, "tar", index.Name, "bytes", index.Size)
			}
			return nil
		}
		if err := f.Truncate(0); err != nil {
			return err
		}
		if err := wait(-1, 0, errors.New("archive SHA-256 mismatch")); err != nil {
			return err
		}
	}
}

type segmentWriter struct {
	file      *os.File
	remaining int64
	overflow  bool
	writeErr  error
}

func (w *segmentWriter) Write(b []byte) (int, error) {
	if int64(len(b)) > w.remaining {
		w.overflow = true
		return 0, errors.New("too many segment bytes")
	}
	n, err := w.file.Write(b)
	w.remaining -= int64(n)
	w.writeErr = err
	return n, err
}
