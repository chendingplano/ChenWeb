package kbhandler

import (
	"context"
	"database/sql"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/ontology/profiles"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

// reviewDocumentFactsLoader adapts the doc-processing extraction fact builders
// (facet observations, object classification, deployment context) to
// profiles.SubjectFactsLoader so a deterministic scope evaluates profile
// applicability against the same facts the extraction routing path uses
// (spec 2026080102 acceptance criterion 9). The review context facts are
// merged separately by the selector, so this loader supplies only the
// per-subject document/object/deployment facts.
//
// VocabularyRelease is the vocabulary release id used to address
// kb.doc_facet_values and the object.class provenance. There is no
// vocabulary-release resolver in the tree yet (the extraction writer path is
// not wired), so the deterministic wiring passes 0, matching the schema
// default; the fact provenance (release id) is therefore absent from the
// subject facts until a resolver exists.
type reviewDocumentFactsLoader struct {
	DB                *sql.DB
	VocabularyRelease int64
}

func (l reviewDocumentFactsLoader) LoadSubjectFacts(ctx context.Context, subject profiles.SelectionSubject) (semrules.FactSet, error) {
	facetObservations, err := (docprocessing.SQLStore{DB: l.DB}).ListFacetObservations(ctx, subject.DocumentID, l.VocabularyRelease)
	if err != nil {
		return nil, err
	}
	objectClasses, err := (docprocessing.ClassificationFactLoader{DB: l.DB}).LoadObjectClassFacts(ctx, subject.DocumentID, l.VocabularyRelease)
	if err != nil {
		return nil, err
	}
	// Deployment context is not derivable from a review-scope request; an
	// empty context yields all-missing deployment facts, consistent with the
	// extraction path when no deployment context is available.
	deployment, err := docprocessing.BuildDeploymentFacts(docprocessing.DeploymentFactContext{})
	if err != nil {
		return nil, err
	}
	return docprocessing.MergeApplicabilityFactSets(
		docprocessing.BuildApplicabilityFactSet(facetObservations),
		objectClasses,
		deployment,
	)
}
