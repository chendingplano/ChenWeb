package docbuilder

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	headingRe  = regexp.MustCompile(`(?m)^#{1,6}\s+(.+?)\s*#*\s*$`)
	frontTitle = regexp.MustCompile(`(?m)^title:\s*["']?(.+?)["']?\s*$`)
)

// Scan walks root recursively and returns every .md and .pdf file (hidden files and
// directories are skipped), sorted by path. Other file types are ignored.
func Scan(root string) ([]Entry, error) {
	var out []Entry
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
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
		kind := ""
		switch strings.ToLower(filepath.Ext(d.Name())) {
		case ".md", ".markdown":
			kind = KindMarkdown
		case ".pdf":
			kind = KindPDF
		default:
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		e := Entry{Path: filepath.ToSlash(rel), Kind: kind, Hash: hex.EncodeToString(sum[:])[:16]}
		e.Title = fileTitle(d.Name())
		if kind == KindMarkdown {
			if t := markdownTitle(string(raw)); t != "" {
				e.Title = t
			}
		}
		out = append(out, e)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func fileTitle(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return strings.TrimSpace(strings.NewReplacer("-", " ", "_", " ").Replace(base))
}

func markdownTitle(src string) string {
	body := src
	if strings.HasPrefix(src, "---\n") {
		if end := strings.Index(src[4:], "\n---"); end >= 0 {
			if m := frontTitle.FindStringSubmatch(src[4 : 4+end]); m != nil {
				return strings.TrimSpace(m[1])
			}
			body = src[4+end+4:]
		}
	}
	if m := headingRe.FindStringSubmatch(body); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}
