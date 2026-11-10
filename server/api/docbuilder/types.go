// Package docbuilder publishes documents as categorised menus. Documents (Markdown and PDF files
// under DOCUMENT_ROOT_DIR) are registered in the database with stable IDs; document groups pick
// members (directories and files), keep a category per document, and bind to one menu location.
// A document always means its highest -v<number> version. See openspec change document-groups.
package docbuilder

// Entry is one document of a group, as the tree shows it.
type Entry struct {
	DocID    int64  `json:"doc_id"`
	Path     string `json:"path"` // current (latest version) file, relative to the root
	Kind     string `json:"kind"` // "markdown" or "pdf"
	Title    string `json:"title"`
	Category string `json:"category"` // "" until classified in this group
}

// TreeNode is a menu node: a category (Children) or a document (DocID set).
type TreeNode struct {
	ID       string     `json:"id"`
	Label    string     `json:"label"`
	Kind     string     `json:"kind,omitempty"`   // document kind for leaves
	DocID    int64      `json:"doc_id,omitempty"` // leaf only
	Children []TreeNode `json:"children,omitempty"`
}

const (
	KindMarkdown = "markdown"
	KindPDF      = "pdf"
	// Uncategorized is the category shown for documents not classified yet in a group.
	Uncategorized = "Uncategorized"
	// IDSequence is the shared ID manager sequence that allocates document IDs.
	IDSequence = "doc_registry_id"
)
