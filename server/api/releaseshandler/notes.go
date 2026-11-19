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

// Notes is one set of release notes: plain text, and the Typst source of the PDF.
type Notes struct {
	Text  string
	Typst string
}

// itemLine formats an item as "<description> (#n)", "(shared#n)", or "[shared] <description>".
func itemLine(it Item) string {
	desc := plain(it.Description)
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

func plain(s string) string { return strings.Join(strings.Fields(s), " ") }

// typstStr writes s as a Typst string literal. All text goes into the PDF this way, so
// characters such as # * _ @ $ < [ in an item can never be read as Typst markup.
func typstStr(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range plain(s) {
		switch {
		case r == '\\' || r == '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20:
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

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

// typstPreamble sets up the page. The fonts are tried in order and Typst falls back to any
// installed font for missing characters, so Chinese uses SimSun/SimHei on the China box
// and PingFang on the Mac, as the doc-review reports do.
const typstPreamble = `#set page(paper: "a4", margin: (x: 2.2cm, y: 2.4cm))
#set text(font: ("Helvetica", "Arial", "PingFang SC", "SimHei", "SimSun"), size: 10pt, lang: %s)
#set heading(numbering: none)
#show heading.where(level: 2): set text(size: 13pt)
#show heading.where(level: 3): set text(size: 11pt)
`

// Render writes the notes for releases (newest first, from InRange) in lang. from is the
// exclusive start of the range (nil = from the first release).
func Render(project string, releases []Release, from *Version, to Version, lang string) Notes {
	l := labelsFor(lang)
	var txt, typ strings.Builder
	title := fmt.Sprintf(l.title, project)
	subtitle := fmt.Sprintf(l.rangeAll, to)
	if from != nil {
		subtitle = fmt.Sprintf(l.rangeFrom, *from, to)
	}
	if len(releases) == 1 && releases[0].Kind == KindBuild {
		subtitle = fmt.Sprintf(l.single, to)
	}
	typstLang := `"en"`
	if lang == "zh-cn" {
		typstLang = `"zh"`
	}
	fmt.Fprintf(&txt, "%s\n%s\n", title, subtitle)
	fmt.Fprintf(&typ, "#set document(title: %s)\n", typstStr(title))
	fmt.Fprintf(&typ, typstPreamble, typstLang)
	fmt.Fprintf(&typ, "#text(size: 17pt, weight: \"bold\", %s)\n\n#text(fill: luma(90), %s)\n", typstStr(title), typstStr(subtitle))
	for _, r := range releases {
		if r.Kind != KindBuild {
			h := lineHeading(r)
			fmt.Fprintf(&txt, "\n== %s ==\n", h)
			fmt.Fprintf(&typ, "\n#heading(level: 2, %s)\n", typstStr(h))
			continue
		}
		meta := r.ReleaseDate
		if r.BuildNumber != "" {
			meta = strings.TrimPrefix(meta+", "+fmt.Sprintf(l.build, r.BuildNumber), ", ")
		}
		head := fmt.Sprintf("%s (%s)", r.Version, meta)
		fmt.Fprintf(&txt, "\n%s\n", head)
		fmt.Fprintf(&typ, "\n#heading(level: 3, %s)\n", typstStr(head))
		if r.Description != "" {
			fmt.Fprintf(&txt, "%s\n", plain(r.Description))
			fmt.Fprintf(&typ, "#text(%s)\n", typstStr(r.Description))
		}
		if len(r.Items) == 0 {
			fmt.Fprintf(&txt, "  %s\n", l.noChanges)
			fmt.Fprintf(&typ, "#text(%s)\n", typstStr(l.noChanges))
			continue
		}
		for _, t := range itemTypes {
			var lines []string
			for _, it := range r.Items {
				if it.ItemType == t {
					lines = append(lines, itemLine(it))
				}
			}
			if len(lines) == 0 {
				continue
			}
			fmt.Fprintf(&txt, "  %s\n", l.types[t])
			fmt.Fprintf(&typ, "\n#strong(%s)\n#list(", typstStr(l.types[t]))
			for i, line := range lines {
				fmt.Fprintf(&txt, "  - %s\n", line)
				if i > 0 {
					typ.WriteString(", ")
				}
				typ.WriteString(typstStr(line))
			}
			typ.WriteString(")\n")
		}
	}
	return Notes{Text: txt.String(), Typst: typ.String()}
}

// PDFRenderer compiles notes Typst source into a PDF file with the typst command,
// reusing an earlier PDF of the same source.
type PDFRenderer struct {
	CacheDir string
	// Compile defaults to `typst compile <src> <dst>`; tests replace it.
	Compile  func(ctx context.Context, src, dst string) error
	LookPath func(string) (string, error)
}

// NewPDFRenderer keeps its PDFs in cacheDir.
func NewPDFRenderer(cacheDir string) *PDFRenderer {
	return &PDFRenderer{CacheDir: cacheDir, LookPath: exec.LookPath}
}

func typstCompile(ctx context.Context, src, dst string) error {
	out, err := exec.CommandContext(ctx, "typst", "compile", src, dst).CombinedOutput()
	if err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("typst compile: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// PDF returns the path of the PDF made from source.
func (p *PDFRenderer) PDF(ctx context.Context, source string) (string, error) {
	sum := sha256.Sum256([]byte(source))
	key := hex.EncodeToString(sum[:])
	dst := filepath.Join(p.CacheDir, key+".pdf")
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	compile := p.Compile
	if compile == nil {
		if _, err := p.LookPath("typst"); err != nil {
			return "", fmt.Errorf("%w: typst is not installed on this machine", ErrPDFUnavailable)
		}
		compile = typstCompile
	}
	if err := os.MkdirAll(p.CacheDir, 0o755); err != nil {
		return "", err
	}
	src := filepath.Join(p.CacheDir, key+".typ")
	if err := os.WriteFile(src, []byte(source), 0o644); err != nil {
		return "", err
	}
	defer os.Remove(src)
	tmp := filepath.Join(p.CacheDir, key+".tmp.pdf")
	if err := compile(ctx, src, tmp); err != nil {
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
