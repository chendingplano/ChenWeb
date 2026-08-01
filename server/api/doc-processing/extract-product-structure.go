package docprocessing

import (
	"context"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// ProductStructureProcessor is a routed post-processing processor. Relation
// extraction and entity-object reconciliation produce its governed inputs, so
// it intentionally has no per-chunk LLM pass of its own.
type ProductStructureProcessor struct{}

func NewProductStructureProcessor() *ProductStructureProcessor                 { return &ProductStructureProcessor{} }
func (p *ProductStructureProcessor) Name() string                              { return "extract_product_structure" }
func (p *ProductStructureProcessor) HandleEvent(context.Context, []byte) error { return nil }
func (p *ProductStructureProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	if ApiTypes.ProjectDBHandle == nil {
		return nil
	}
	return HarvestProductStructureFromRelations(ctx, ApiTypes.ProjectDBHandle, recordID, assertions.DecisionCandidateStore{DB: ApiTypes.ProjectDBHandle})
}
