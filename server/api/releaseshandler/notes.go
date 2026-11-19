package releaseshandler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/docbuilder"
)

// ErrPDFUnavailable means this machine lacks the tools to make a PDF.
var ErrPDFUnavailable = errors.New("PDF rendering unavailable")

// noteLabels holds the server-generated words of the notes per language.
type noteLabels struct {
	title, single, rangeFrom, rangeAll, build, noChanges string
	types                                                map[string]string
}

var labels = map[string]noteLabels{
	"en": {
		title: "%s release notes", single: "Release %s", rangeFrom: "Changes after %s up to %s",
		rangeAll: "Changes up to %s", build: "build %s", noChanges: "No changes were recorded.",
		types: map[string]string{"new feature": "New features", "improvement": "Improvements", "bug fix": "Bug fixes"},
	},
	"zh-cn": {
		title: "%s 发布说明", single: "版本 %s", rangeFrom: "%s 之后至 %s 的变更",
		rangeAll: "截至 %s 的变更", build: "构建 %s", noChanges: "未记录变更。",
		types: map[string]string{"new feature": "新功能", "improvement": "改进", "bug fix": "缺陷修复"},
	},
}

// Langs are the languages the notes are written in.
var Langs = []string{"en", "zh-cn"}

func labelsFor(lang string) noteLabels {
	if l, ok := labels[lang]; ok {
		return l
	}
	return labels["en"]
}

// InRange returns the released releases after from (nil = from the first) up to and
// including to, keeping history's newest-first order.
func InRange(history []Release, from *Version, to Version) []Release {
	out := []Release{}
	for _, r := range history {
		if r.Status != StatusReleased || r.V.Compare(to) > 0 || (from != nil && r.V.Compare(*from) <= 0) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Notes is one set of release notes in both forms.
type Notes struct {
	Markdown string
	Text     string
}

// itemLine formats an item as "<description> (#n)", "(shared#n)", or "[shared] <description>".
func itemLine(it Item, escape func(string) string) string {
	desc := escape(it.Description)
	switch {
	case it.Component == "shared" && it.Issue != "":
		return fmt.Sprintf("%s (shared#%s)", desc, it.Issue)
	case it.Component == "shared":
		return "[shared] " + desc
	case it.Issue != "":
		return fmt.Sprintf("%s (#%s)", desc, it.Issue)
	}
	return desc
}

var mdEscaper = strings.NewReplacer(`\`, `\\`, "`", "\\`", `*`, `\*`, `_`, `\_`, `[`, `\[`, `]`, `\]`,
	`<`, `\<`, `>`, `\>`, `#`, `\#`, `|`, `\|`)

func mdEscape(s string) string { return mdEscaper.Replace(strings.Join(strings.Fields(s), " ")) }
func plain(s string) string    { return strings.Join(strings.Fields(s), " ") }

func lineHeading(r Release) string {
	name := fmt.Sprintf("%02d.%02d", r.V.Major, r.V.Minor)
	if r.Kind == KindMajor {
		name = fmt.Sprintf("%02d", r.V.Major)
	}
	if r.Description == "" {
		return name
	}
	return name + " — " + plain(r.Description)
}

// Render writes the notes for releases (newest first, from InRange) in lang. from is the
// exclusive start of the range (nil = from the first release).
func Render(project string, releases []Release, from *Version, to Version, lang string) Notes {
	l := labelsFor(lang)
	var md, txt strings.Builder
	title := fmt.Sprintf(l.title, project)
	subtitle := fmt.Sprintf(l.rangeAll, to)
	if from != nil {
		subtitle = fmt.Sprintf(l.rangeFrom, *from, to)
	}
	if len(releases) == 1 && releases[0].Kind == KindBuild {
		subtitle = fmt.Sprintf(l.single, to)
	}
	fmt.Fprintf(&md, "---\ntitle: %q\n---\n\n%s\n", title, mdEscape(subtitle))
	fmt.Fprintf(&txt, "%s\n%s\n", title, subtitle)
	for _, r := range releases {
		if r.Kind != KindBuild {
			h := lineHeading(r)
			fmt.Fprintf(&md, "\n## %s\n", mdEscape(h))
			fmt.Fprintf(&txt, "\n== %s ==\n", h)
			continue
		}
		meta := r.ReleaseDate
		if r.BuildNumber != "" {
			meta = strings.TrimPrefix(meta+", "+fmt.Sprintf(l.build, r.BuildNumber), ", ")
		}
		fmt.Fprintf(&md, "\n### %s (%s)\n", r.Version, mdEscape(meta))
		fmt.Fprintf(&txt, "\n%s (%s)\n", r.Version, meta)
		if r.Description != "" {
			fmt.Fprintf(&md, "\n%s\n", mdEscape(r.Description))
			fmt.Fprintf(&txt, "%s\n", plain(r.Description))
		}
		if len(r.Items) == 0 {
			fmt.Fprintf(&md, "\n%s\n", l.noChanges)
			fmt.Fprintf(&txt, "  %s\n", l.noChanges)
			continue
		}
		for _, typ := range itemTypes {
			first := true
			for _, it := range r.Items {
				if it.ItemType != typ {
					continue
				}
				if first {
					fmt.Fprintf(&md, "\n**%s**\n\n", l.types[typ])
					fmt.Fprintf(&txt, "  %s\n", l.types[typ])
					first = false
				}
				fmt.Fprintf(&md, "- %s\n", itemLine(it, mdEscape))
				fmt.Fprintf(&txt, "  - %s\n", itemLine(it, plain))
			}
		}
	}
	return Notes{Markdown: md.String(), Text: txt.String()}
}

// PDFRenderer turns notes Markdown into a PDF file, reusing an earlier PDF of the same text.
type PDFRenderer struct {
	CacheDir string
	Engine   string
	Font     string
	// Convert defaults to docbuilder.PandocConvert(Engine, Font); tests replace it.
	Convert  func(ctx context.Context, src, dst string) error
	LookPath func(string) (string, error)
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// NewPDFRenderer uses Document Builder's PDF settings (DOCUMENT_PDF_ENGINE, DOCUMENT_PDF_FONT).
func NewPDFRenderer(cacheDir string) *PDFRenderer {
	return &PDFRenderer{CacheDir: cacheDir, Engine: envOr("DOCUMENT_PDF_ENGINE", "typst"),
		Font: envOr("DOCUMENT_PDF_FONT", "Helvetica"), LookPath: exec.LookPath}
}

// PDF returns the path of a PDF made from markdown.
func (p *PDFRenderer) PDF(ctx context.Context, markdown string) (string, error) {
	sum := sha256.Sum256([]byte(p.Engine + "\x00" + p.Font + "\x00" + markdown))
	key := hex.EncodeToString(sum[:])
	dst := filepath.Join(p.CacheDir, key+".pdf")
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	convert := p.Convert
	if convert == nil {
		for _, tool := range []string{"pandoc", p.Engine} {
			if _, err := p.LookPath(tool); err != nil {
				return "", fmt.Errorf("%w: %s is not installed on this machine", ErrPDFUnavailable, tool)
			}
		}
		convert = docbuilder.PandocConvert(p.Engine, p.Font)
	}
	if err := os.MkdirAll(p.CacheDir, 0o755); err != nil {
		return "", err
	}
	src := filepath.Join(p.CacheDir, key+".md")
	if err := os.WriteFile(src, []byte(markdown), 0o644); err != nil {
		return "", err
	}
	defer os.Remove(src)
	tmp := filepath.Join(p.CacheDir, key+".tmp.pdf")
	if err := convert(ctx, src, tmp); err != nil {
		return "", err
	}
	return dst, os.Rename(tmp, dst)
}

// PreviousBuild returns the newest build release older than v (history is newest first),
// or nil when v is the first build. Notes for one release run from there, so the first
// build of a new major or minor also shows that major's or minor's heading.
func PreviousBuild(history []Release, v Version) *Version {
	for _, r := range history {
		if r.Kind == KindBuild && r.Status == StatusReleased && r.V.Compare(v) < 0 {
			prev := r.V
			return &prev
		}
	}
	return nil
}
