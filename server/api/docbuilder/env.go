package docbuilder

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/idmanager"
)

// Environment variables:
//
//	DOCUMENT_ROOT_DIR     required, absolute: the Document Root Directory
//	DOCUMENT_ARCHIVE_DIR  default <root>/.doc-archive: where deleted documents are moved
//	DOCUMENT_PDF_DIR      default <root>/.doc-pdf: cached PDF exports
//	DOCUMENT_PDF_ENGINE   default typst: pandoc --pdf-engine for Markdown → PDF
//	DOCUMENT_PDF_FONT     default Helvetica: main font name (must be installed; Chinese falls back automatically)
//	DOCBUILDER_MODEL      default deepseek-flash-processor: .models.toml profile for classification
//	DOCBUILDER_PROMPT     default prompt-docbuilder-classify-v1.md
const (
	defaultModel  = "deepseek-flash-processor"
	defaultPrompt = "prompt-docbuilder-classify-v1.md"
)

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// NewStoreFromEnv builds the store from the environment. Document IDs come from the shared ID
// manager on sharedDB (sequence IDSequence, provisioned here; safe to repeat).
func NewStoreFromEnv(ctx context.Context, projectDB, sharedDB *sql.DB, requester string, logf func(string, ...any)) (*Store, error) {
	root := strings.TrimSpace(os.Getenv("DOCUMENT_ROOT_DIR"))
	if root == "" {
		return nil, fmt.Errorf("DOCUMENT_ROOT_DIR is not set")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("DOCUMENT_ROOT_DIR %q is not a directory", root)
	}
	if sharedDB == nil {
		return nil, fmt.Errorf("shared database is unavailable (needed for document IDs)")
	}
	ids, err := idmanager.New(sharedDB, requester)
	if err != nil {
		return nil, fmt.Errorf("create ID manager: %w", err)
	}
	if err := ids.Provision(ctx, idmanager.Sequence{Name: IDSequence, Desc: "Document registry IDs (kb.doc_documents.doc_id)"}); err != nil {
		return nil, fmt.Errorf("provision %s: %w", IDSequence, err)
	}
	return &Store{
		DB:         projectDB,
		Root:       root,
		ArchiveDir: envOr("DOCUMENT_ARCHIVE_DIR", filepath.Join(root, ".doc-archive")),
		PDFDir:     envOr("DOCUMENT_PDF_DIR", filepath.Join(root, ".doc-pdf")),
		NextID:     func(ctx context.Context) (int64, error) { return ids.Next(ctx, IDSequence) },
		Convert:    PandocConvert(envOr("DOCUMENT_PDF_ENGINE", "typst"), envOr("DOCUMENT_PDF_FONT", "Helvetica")),
		Logf:       logf,
	}, nil
}

// NewClassifierFromEnv builds the LLM classifier (model profile and prompt from the environment).
// The model is returned as the profile name that is saved with each category.
func NewClassifierFromEnv(model string, logger ApiTypes.JimoLogger) (Classifier, string, error) {
	if model == "" {
		model = envOr("DOCBUILDER_MODEL", defaultModel)
	}
	client, modelName, err := docprocessing.BuildReviewerToolClient(model, logger)
	if err != nil {
		return nil, "", err
	}
	prompt, _, _, err := docprocessing.LoadPromptByRef(envOr("DOCBUILDER_PROMPT", defaultPrompt))
	if err != nil {
		return nil, "", err
	}
	return &LLMClassifier{Client: client, Model: modelName, Prompt: prompt}, model, nil
}
