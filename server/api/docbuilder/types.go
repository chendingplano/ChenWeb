// Package docbuilder turns a directory of Markdown and PDF files into a categorised document
// tree. A manifest (<dir>/.docbuilder/manifest.json) records each file's category; categories are
// assigned once by an LLM (Classify) and then only read, so serving never calls a model.
package docbuilder

// Entry is one document found under a source directory.
type Entry struct {
	Path     string `json:"path"`     // slash-separated, relative to the source directory
	Kind     string `json:"kind"`     // "markdown" or "pdf"
	Title    string `json:"title"`    // first heading, front-matter title, or the file name
	Hash     string `json:"hash"`     // short sha256 of the content
	Category string `json:"category"` // "" until classified
}

// Manifest is the saved result of scanning and classifying one directory.
type Manifest struct {
	Version int     `json:"version"`
	Model   string  `json:"model,omitempty"` // model profile that classified the entries
	Entries []Entry `json:"entries"`
}

// TreeNode is a menu node: a category or folder (Children) or a document (Path set).
type TreeNode struct {
	ID       string     `json:"id"`
	Label    string     `json:"label"`
	Kind     string     `json:"kind,omitempty"` // document kind for leaves
	Path     string     `json:"path,omitempty"` // leaf only
	Children []TreeNode `json:"children,omitempty"`
}

const (
	KindMarkdown = "markdown"
	KindPDF      = "pdf"
	// Uncategorized is the category shown for entries the classifier has not placed yet.
	Uncategorized = "Uncategorized"
	manifestDir   = ".docbuilder"
	manifestFile  = "manifest.json"
)
