package docsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/pathutil"
)

type fileChange struct {
	Target string
	Backup string
	Before bool
	Source string `json:"-"`
}

type fileJournal struct {
	Site     string
	SourceID int64
	Kind     string
	Digest   string
	SeenAt   string
	Changes  []fileChange
	filename string
}

func (j *fileJournal) prepare(inbox string) error {
	file, err := os.CreateTemp(inbox, ".preparing-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	for i := range j.Changes {
		change := &j.Changes[i]
		st, err := os.Lstat(change.Target)
		if err == nil && !st.Mode().IsRegular() {
			file.Close()
			return fmt.Errorf("artifact target %q is not a regular file", change.Target)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			file.Close()
			return err
		}
		change.Before = err == nil
		change.Backup = filepath.Join(filepath.Dir(change.Target), fmt.Sprintf(".docsync-backup-%s-%d", filepath.Base(file.Name()), i))
	}
	err = json.NewEncoder(file).Encode(j)
	err = errors.Join(err, file.Sync(), file.Close())
	if err != nil {
		return err
	}
	j.filename = file.Name() + ".journal"
	if err := os.Rename(file.Name(), j.filename); err != nil {
		return err
	}
	return syncDirectory(inbox)
}

func (j fileJournal) place(ctx context.Context) error {
	for _, change := range j.Changes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(change.Target), 0o750); err != nil {
			return err
		}
		if change.Before {
			if err := os.Link(change.Target, change.Backup); err != nil {
				return err
			}
			if err := syncDirectory(filepath.Dir(change.Target)); err != nil {
				return err
			}
		}
		if change.Source == "" {
			if err := ignoreMissing(os.Remove(change.Target)); err != nil {
				return err
			}
		} else if err := copyArtifact(ctx, change.Source, change.Target, change.Backup+".tmp"); err != nil {
			return err
		}
		if err := syncDirectory(filepath.Dir(change.Target)); err != nil {
			return err
		}
	}
	return nil
}

func (j fileJournal) finish(committed bool) error {
	for _, change := range j.Changes {
		if err := ignoreMissing(os.Remove(change.Backup + ".tmp")); err != nil {
			return err
		}
		if !committed && change.Before {
			if err := ignoreMissing(os.Rename(change.Backup, change.Target)); err != nil {
				return err
			}
		} else {
			if err := ignoreMissing(os.Remove(change.Backup)); err != nil {
				return err
			}
			if !committed {
				if err := ignoreMissing(os.Remove(change.Target)); err != nil {
					return err
				}
			}
		}
		if err := ignoreMissing(syncDirectory(filepath.Dir(change.Target))); err != nil {
			return err
		}
	}
	if err := ignoreMissing(os.Remove(j.filename)); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(j.filename))
}

func recoverFiles(ctx context.Context, db *sql.DB, inbox string) error {
	paths, err := filepath.Glob(filepath.Join(inbox, ".preparing-*.journal"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var j fileJournal
		if err := json.Unmarshal(raw, &j); err != nil {
			return err
		}
		j.filename = path
		committed, err := journalCommitted(ctx, db, j)
		if err != nil {
			return err
		}
		if err := j.finish(committed); err != nil {
			return err
		}
	}
	return nil
}

func journalCommitted(ctx context.Context, db *sql.DB, j fileJournal) (bool, error) {
	var committed bool
	err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM kb.doc_sync_applied
		WHERE site=$1 AND source_id=$2 AND processor=$3 AND digest=$4 AND source_time=$5)`,
		j.Site, j.SourceID, j.Kind, j.Digest, j.SeenAt).Scan(&committed)
	return committed, err
}

func ignoreMissing(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func syncDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

func copyArtifact(ctx context.Context, source, target, temp string) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, cancellableReader{ctx, src})
	if err == nil {
		err = dst.Sync()
	}
	if err = errors.Join(err, dst.Close()); err != nil {
		return err
	}
	return os.Rename(temp, target)
}

type cancellableReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r cancellableReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func localRelativePath(path string) (string, error) {
	home := pathutil.DataHomeDir()
	if !filepath.IsAbs(home) {
		return "", errors.New("DATA_HOME_DIR must be an absolute path")
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("document sync artifact root must be inside DATA_HOME_DIR")
	}
	return rel, nil
}
