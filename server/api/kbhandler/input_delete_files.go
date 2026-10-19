package kbhandler

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/pathutil"
)

// inputDeletePlan lists every physical location a record's delete may touch. It is built and
// checked before the database transaction changes anything, so a refused plan leaves the
// record intact.
type inputDeletePlan struct {
	PDFPath string   // uploaded PDF in the data home; archived by DeleteInput, deleted by CleanInputArtifacts
	Files   []string // backup copy and result file
	Dirs    []string // record artifact folders, each named after the record id
}

// filesWithPDF returns every planned file, including the uploaded PDF.
func (p inputDeletePlan) filesWithPDF() []string {
	if p.PDFPath == "" {
		return p.Files
	}
	return append([]string{p.PDFPath}, p.Files...)
}

// inputDeleteReport records what the file step did. Failures are returned to the caller and
// written to the audit log; nothing is dropped silently.
type inputDeleteReport struct {
	DeletedFiles []string `json:"deleted_files"`
	ArchivedFile string   `json:"archived_file"`
	MissingFiles []string `json:"missing_files"`
	Failures     []string `json:"failures"`
}

func newInputDeleteReport() inputDeleteReport {
	return inputDeleteReport{
		DeletedFiles: []string{},
		MissingFiles: []string{},
		Failures:     []string{},
	}
}

// planInputDeletePaths resolves the files and folders to remove for a record. A folder is only
// accepted when it is the record's directory under a configured artifact root, so a stored
// path cannot cause removal of a shared or unrelated folder.
func planInputDeletePaths(recordID int64, assets inputDeleteAssets) (inputDeletePlan, error) {
	recordKey := strconv.FormatInt(recordID, 10)
	plan := inputDeletePlan{PDFPath: pathutil.ResolveDataHomePath(assets.FileName)}

	seenFiles := map[string]struct{}{}
	addFile := func(path string) {
		if path == "" || path == plan.PDFPath {
			return
		}
		if _, ok := seenFiles[path]; ok {
			return
		}
		seenFiles[path] = struct{}{}
		plan.Files = append(plan.Files, path)
	}
	addFile(pathutil.ResolveBackupPath(assets.BackupFileName))
	addFile(pathutil.ResolveDataHomePath(assets.ResultFileName))

	seenDirs := map[string]struct{}{}
	addDir := func(dir string) error {
		if strings.TrimSpace(dir) == "" {
			return nil
		}
		dir = filepath.Clean(strings.TrimSpace(dir))
		if filepath.Base(dir) != recordKey {
			return fmt.Errorf("refusing to remove folder %s: its name is not record %s", dir, recordKey)
		}
		if _, ok := seenDirs[dir]; ok {
			return nil
		}
		seenDirs[dir] = struct{}{}
		plan.Dirs = append(plan.Dirs, dir)
		return nil
	}

	artifactRoot := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactRoot == "" {
		return inputDeletePlan{}, fmt.Errorf("ARTIFACT_DIR is not set, so the artifact folder of record %s cannot be located", recordKey)
	}
	groupKey := strconv.FormatInt(recordID/1000, 10)
	for _, dir := range []string{
		filepath.Join(artifactRoot, groupKey, recordKey),
		filepath.Join(artifactRoot, "Artifacts", groupKey, recordKey),
	} {
		if err := addDir(dir); err != nil {
			return inputDeletePlan{}, err
		}
	}
	if resolved := pathutil.ResolveDataHomePath(assets.ResultFileName); resolved != "" {
		dir := filepath.Dir(resolved)
		absoluteDir, err := filepath.Abs(dir)
		if err != nil {
			return inputDeletePlan{}, err
		}
		allowed := append([]string{}, plan.Dirs...)
		if dataHome := pathutil.DataHomeDir(); dataHome != "" {
			allowed = append(allowed, filepath.Join(dataHome, "Artifacts", groupKey, recordKey))
		}
		approved := false
		for _, candidate := range allowed {
			absoluteCandidate, err := filepath.Abs(candidate)
			if err != nil {
				return inputDeletePlan{}, err
			}
			if absoluteDir == absoluteCandidate {
				approved = true
				break
			}
		}
		if !approved {
			return inputDeletePlan{}, fmt.Errorf("refusing to remove folder %s: not the configured artifact folder for record %s", dir, recordKey)
		}
		if err := addDir(dir); err != nil {
			return inputDeletePlan{}, err
		}
	}
	return plan, nil
}

// archiveInputPDF moves the uploaded PDF into ARCHIVE_DIR/<group>/<record id>/ and returns the
// archived path. A PDF that is already missing is reported with missing=true and is not an error.
func archiveInputPDF(archiveRoot string, recordID int64, src string) (dst string, missing bool, err error) {
	if src == "" {
		return "", false, nil
	}
	info, statErr := os.Lstat(src)
	if os.IsNotExist(statErr) {
		return "", true, nil
	}
	if statErr != nil {
		return "", false, statErr
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("PDF source is not a regular file: %s", src)
	}
	dir := filepath.Join(archiveRoot, strconv.FormatInt(recordID/1000, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, fmt.Errorf("create archive folder %s: %w", dir, err)
	}
	for {
		dst, err = uniqueArchivePath(dir, filepath.Base(src))
		if err != nil {
			return "", false, err
		}
		if err := moveFile(src, dst); err != nil {
			if errors.Is(err, fs.ErrExist) {
				continue // Another request reserved this name first.
			}
			return "", false, fmt.Errorf("archive %s to %s: %w", src, dst, err)
		}
		return dst, false, nil
	}
}

// uniqueArchivePath returns dir/name, or dir/name_N.ext when that name is taken.
func uniqueArchivePath(dir, name string) (string, error) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	candidate := filepath.Join(dir, name)
	for i := 1; ; i++ {
		_, err := os.Lstat(candidate)
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("inspect archive destination %s: %w", candidate, err)
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s_%d%s", stem, i, ext))
	}
}

// moveFile reserves dst without overwriting another archive or a newly uploaded source on
// restore. Hard links avoid copying on the same filesystem; other filesystems use an exclusive copy.
func moveFile(src, dst string) error {
	if err := os.Link(src, dst); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return err
		}
		if err := copyFileSynced(src, dst); err != nil {
			return err
		}
	}
	if err := os.Remove(src); err != nil {
		return removeFailedArchive(dst, fmt.Errorf("remove source after copy: %w", err))
	}
	return nil
}

func copyFileSynced(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		return removeFailedArchive(dst, errors.Join(err, out.Close()))
	}
	if err := out.Sync(); err != nil {
		return removeFailedArchive(dst, errors.Join(err, out.Close()))
	}
	if err := out.Close(); err != nil {
		return removeFailedArchive(dst, err)
	}
	return nil
}

func removeFailedArchive(path string, cause error) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return errors.Join(cause, fmt.Errorf("remove incomplete archive %s: %w", path, err))
	}
	return cause
}

// restoreArchivedPDF moves an archived PDF back to its original path. It is used when the delete
// does not commit after the PDF was already archived.
func restoreArchivedPDF(archived, original string) error {
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		return err
	}
	return moveFile(archived, original)
}

// deleteInputFiles removes the planned files. A file that is already gone is reported as missing.
func deleteInputFiles(paths []string, report *inputDeleteReport) {
	for _, path := range paths {
		if path == "" {
			continue
		}
		err := os.Remove(path)
		switch {
		case err == nil:
			report.DeletedFiles = append(report.DeletedFiles, path)
		case os.IsNotExist(err):
			report.MissingFiles = append(report.MissingFiles, path)
		default:
			report.Failures = append(report.Failures, fmt.Sprintf("remove file %s: %v", path, err))
		}
	}
}

// deleteInputArtifactDirs records each successful unlink, including when a sibling fails.
// Lstat ensures directory symlinks are removed without following them.
func deleteInputArtifactDirs(dirs []string, report *inputDeleteReport) {
	for _, dir := range dirs {
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			report.MissingFiles = append(report.MissingFiles, dir+string(filepath.Separator))
			continue
		}
		if err != nil {
			report.Failures = append(report.Failures, fmt.Sprintf("inspect folder %s: %v", dir, err))
			continue
		}
		if !info.IsDir() {
			deleteInputFiles([]string{dir}, report)
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			report.Failures = append(report.Failures, fmt.Sprintf("list folder %s: %v", dir, err))
			continue
		}
		for _, entry := range entries {
			deleteInputArtifactDirs([]string{filepath.Join(dir, entry.Name())}, report)
		}
		if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
			report.Failures = append(report.Failures, fmt.Sprintf("remove folder %s: %v", dir, err))
		}
	}
}

func listRegularFiles(dir string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
