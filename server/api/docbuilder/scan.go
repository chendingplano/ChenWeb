package docbuilder

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	headingRe  = regexp.MustCompile(`(?m)^#{1,6}\s+(.+?)\s*#*\s*$`)
	frontTitle = regexp.MustCompile(`(?m)^title:\s*["']?(.+?)["']?\s*$`)
)

// kindOf returns the document kind of a file name, or "" when it is not a supported type.
func kindOf(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return KindMarkdown
	case ".pdf":
		return KindPDF
	}
	return ""
}

// hashFile returns the hex sha256 of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// titleOf reads a document's title: front-matter title, else first heading, else the file name.
func titleOf(path, kind string) string {
	t := fileTitle(filepath.Base(path))
	if kind != KindMarkdown {
		return t
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return t
	}
	if h := markdownTitle(string(raw)); h != "" {
		return h
	}
	return t
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
