package terminologyresourcehandler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/modules"
	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

// quantityModuleID is the 4a module the QUDT approve flow governs.
const quantityModuleID = "quantity"

// qudtGovernanceOutcome reports what the governed-term write (and, if any
// content was pending, the release+activate step) did for one Approve
// request. See openspec/changes/external-resource-approve-ontology-terms.
type qudtGovernanceOutcome struct {
	OK               bool   `json:"ok"`
	TermsInserted    int    `json:"terms_inserted,omitempty"`
	LabelsInserted   int    `json:"labels_inserted,omitempty"`
	MappingsInserted int    `json:"mappings_inserted,omitempty"`
	ReleaseVersion   string `json:"release_version,omitempty"`
	Error            string `json:"error,omitempty"`
}

// writeQUDTTermsFresh runs Step A for a fresh (not already-imported-identical)
// approve: the keyword-lexicon import and the governed quantity/unit/dimension
// term writes share one transaction, so both commit together or neither does
// (design.md Decision 3).
func writeQUDTTermsFresh(ctx context.Context, db *sql.DB, manifestPath, by string) (terminology.ImportResult, qudtGovernanceOutcome) {
	list, err := classifyQUDTTermsFromManifest(manifestPath)
	if err != nil {
		return terminology.ImportResult{}, qudtGovernanceOutcome{Error: err.Error()}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return terminology.ImportResult{}, qudtGovernanceOutcome{Error: err.Error()}
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	result, importErr := (terminology.Runner{DB: tx}).Import(ctx, manifestPath)
	if importErr != nil {
		return terminology.ImportResult{}, qudtGovernanceOutcome{Error: importErr.Error()}
	}

	written, err := writeQUDTOntologyTerms(ctx, tx, list, by)
	if err != nil {
		return terminology.ImportResult{}, qudtGovernanceOutcome{Error: err.Error()}
	}

	if err := tx.Commit(); err != nil {
		return terminology.ImportResult{}, qudtGovernanceOutcome{Error: err.Error()}
	}
	committed = true
	return result, written
}

// writeQUDTTermsReplay runs the governed-term write on its own for an
// already-imported-identical approve: the keyword-lexicon side is already
// committed from a prior request, so only this write needs a transaction here.
func writeQUDTTermsReplay(ctx context.Context, db *sql.DB, manifestPath, by string) qudtGovernanceOutcome {
	list, err := classifyQUDTTermsFromManifest(manifestPath)
	if err != nil {
		return qudtGovernanceOutcome{Error: err.Error()}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return qudtGovernanceOutcome{Error: err.Error()}
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	written, err := writeQUDTOntologyTerms(ctx, tx, list, by)
	if err != nil {
		return qudtGovernanceOutcome{Error: err.Error()}
	}
	if err := tx.Commit(); err != nil {
		return qudtGovernanceOutcome{Error: err.Error()}
	}
	committed = true
	return written
}

// classifyQUDTTermsFromManifest re-reads and re-verifies the approved local
// manifest (a second, cheap local-file checksum verification, not a
// re-download) to get at the same qudt-all.ttl content terminology.Runner.Import
// parses internally, since Import does not expose the parsed artifact to its
// caller.
func classifyQUDTTermsFromManifest(manifestPath string) ([]terminology.QUDTImportedTerm, error) {
	_, artifacts, err := terminology.ParseAndVerifyManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("re-verify manifest for ontology write: %w", err)
	}
	if len(artifacts) != 1 {
		return nil, errors.New("qudt manifest must have exactly one artifact")
	}
	resources, err := terminology.ParseQUDTGraph(artifacts[0].Content)
	if err != nil {
		return nil, fmt.Errorf("parse qudt artifact for ontology write: %w", err)
	}
	return terminology.ClassifyQUDTTerms(resources), nil
}

// writeQUDTOntologyTerms batch-inserts new quantity/unit/dimension terms
// (existing ones are silently skipped), then the preferred/symbol labels and
// exact-IRI mappings of only the terms just inserted, all against tx.
func writeQUDTOntologyTerms(ctx context.Context, tx *sql.Tx, list []terminology.QUDTImportedTerm, by string) (qudtGovernanceOutcome, error) {
	termRows := make([]terms.Term, 0, len(list))
	for _, it := range list {
		termRows = append(termRows, terms.Term{
			TermID: it.TermID, TermKind: it.Kind, ModuleID: quantityModuleID,
			Definition: "QUDT import; source " + it.SourceIRI, Status: "approved",
			CreateBy: by, ModifyBy: by,
		})
	}
	insertedTerms, err := (terms.TermStore{DB: tx}).CreateTermsBatch(ctx, termRows)
	if err != nil {
		return qudtGovernanceOutcome{}, fmt.Errorf("insert quantity terms: %w", err)
	}

	byTermID := make(map[string]terminology.QUDTImportedTerm, len(list))
	for _, it := range list {
		byTermID[it.TermID] = it
	}

	var labelRows []terms.TermLabel
	var mappingRows []terms.Mapping
	for _, t := range insertedTerms {
		it, ok := byTermID[t.TermID]
		if !ok {
			continue
		}
		if it.PrefLabel != "" {
			labelRows = append(labelRows, terms.TermLabel{
				TermID: it.TermID, Label: it.PrefLabel, Lang: "en", LabelRole: "prefLabel",
				Status: "approved", CreateBy: by, ModifyBy: by,
			})
		}
		if it.Symbol != "" {
			labelRows = append(labelRows, terms.TermLabel{
				TermID: it.TermID, Label: it.Symbol, Lang: "en", LabelRole: "altLabel",
				Status: "approved", CreateBy: by, ModifyBy: by,
			})
		}
		mappingRows = append(mappingRows, terms.Mapping{
			MappingID: "quantity:map_" + strings.TrimPrefix(it.TermID, "quantity:"), FromTermID: it.TermID,
			ToIRI: it.SourceIRI, Relation: "exact", ApprovalStatus: "approved",
			ModuleID: quantityModuleID, Status: "approved",
			CreateBy: by, ModifyBy: by,
		})
	}

	insertedLabels, err := (terms.LabelStore{DB: tx}).CreateLabelsBatch(ctx, labelRows)
	if err != nil {
		return qudtGovernanceOutcome{}, fmt.Errorf("insert quantity labels: %w", err)
	}
	insertedMappings, err := (terms.MappingStore{DB: tx}).CreateMappingsBatch(ctx, mappingRows)
	if err != nil {
		return qudtGovernanceOutcome{}, fmt.Errorf("insert quantity mappings: %w", err)
	}

	return qudtGovernanceOutcome{
		OK: true, TermsInserted: len(insertedTerms),
		LabelsInserted: len(insertedLabels), MappingsInserted: len(insertedMappings),
	}, nil
}

// releaseQUDTIfPending runs Step B: ensure the quantity module is registered,
// and if it has any content still at status='approved' -- whether from this
// call or a stranded prior write whose release step previously failed --
// create and activate a new release (design.md Decision 4). It uses db
// directly, not the Step A transaction: modules.ModuleStore and
// modules.ReleaseStore manage their own transactions internally and cannot
// join an externally supplied one. Returns "" (no error) if nothing is
// pending.
func releaseQUDTIfPending(ctx context.Context, db *sql.DB, by string) (string, error) {
	ms := modules.ModuleStore{DB: db}
	if _, err := ms.GetModule(ctx, quantityModuleID); err != nil {
		if _, err := ms.CreateModule(ctx, modules.Module{
			ModuleID: quantityModuleID, Title: "Quantity kinds, units, and dimensions (QUDT)",
			Owner: "platform", DependsOn: []string{"core"}, Status: "active",
			CreateBy: by, ModifyBy: by,
		}); err != nil {
			return "", fmt.Errorf("register quantity module: %w", err)
		}
	}

	var pending bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM kb.ontology_terms WHERE module_id = $1 AND status = 'approved')`,
		quantityModuleID,
	).Scan(&pending); err != nil {
		return "", fmt.Errorf("check pending quantity content: %w", err)
	}
	if !pending {
		return "", nil
	}

	rs := modules.ReleaseStore{DB: db}
	version, err := rs.NextPatchVersion(ctx, quantityModuleID)
	if err != nil {
		return "", fmt.Errorf("compute next quantity release version: %w", err)
	}
	rel, err := rs.CreateRelease(ctx, quantityModuleID, version, by)
	if err != nil {
		return "", fmt.Errorf("release quantity: %w", err)
	}
	if _, err := rs.Activate(ctx, quantityModuleID, rel.ID, by); err != nil {
		return "", fmt.Errorf("activate quantity release %s: %w", version, err)
	}
	return version, nil
}
