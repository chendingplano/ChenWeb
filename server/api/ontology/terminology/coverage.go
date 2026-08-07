// Package terminology contains read-only tooling for measuring a pilot
// corpus against reviewed terminology releases. It inventories evidence; it
// never creates concepts, mappings, or approval decisions.
package terminology

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

const AcceptanceSchemaVersion = 1

type CorpusRef struct {
	ArtifactType string   `json:"artifact_type"`
	ArtifactIDs  []string `json:"artifact_ids,omitempty"`
}

type SeedRelease struct {
	Source  string `json:"source"`
	Release string `json:"release"`
}

type Acceptance struct {
	SchemaVersion     int         `json:"schema_version"`
	Scope             string      `json:"scope"`
	Corpus            []CorpusRef `json:"corpus"`
	TargetCoverage    float64     `json:"target_coverage"`
	RiskTerms         []string    `json:"risk_terms,omitempty"`
	Approver          string      `json:"approver"`
	TargetSeedRelease SeedRelease `json:"target_seed_release"`
}

func (a Acceptance) Validate() error {
	if a.SchemaVersion != AcceptanceSchemaVersion {
		return fmt.Errorf("unsupported schema_version %d", a.SchemaVersion)
	}
	if strings.TrimSpace(a.Scope) == "" {
		return fmt.Errorf("scope is required")
	}
	if len(a.Corpus) == 0 {
		return fmt.Errorf("corpus is required")
	}
	for i, ref := range a.Corpus {
		if strings.TrimSpace(ref.ArtifactType) == "" {
			return fmt.Errorf("corpus[%d].artifact_type is required", i)
		}
	}
	if a.TargetCoverage < 0 || a.TargetCoverage > 1 {
		return fmt.Errorf("target_coverage must be between 0 and 1")
	}
	if strings.TrimSpace(a.Approver) == "" {
		return fmt.Errorf("approver is required")
	}
	if strings.TrimSpace(a.TargetSeedRelease.Source) == "" {
		return fmt.Errorf("target_seed_release.source is required")
	}
	return nil
}

type CoverageQuery struct {
	Scope             string
	Corpus            []CorpusRef
	TargetSeedRelease SeedRelease
}

type Reader interface {
	Load(context.Context, CoverageQuery) (CorpusData, error)
}

type ConceptRecord struct {
	ConceptID      string
	PrefLabel      string
	Scope          string
	Status         string
	ExactAuthority bool
}

type SurfaceRecord struct {
	ConceptID string
	Surface   string
	NormKey   string
	Lang      string
	Scope     string
}

type OccurrenceRecord struct {
	OccurrenceID     int64
	ArtifactType     string
	ArtifactID       string
	ConceptID        string
	NormKey          string
	Scope            string
	ResolutionStatus string
}

type CorpusData struct {
	Concepts    []ConceptRecord
	Surfaces    []SurfaceRecord
	Occurrences []OccurrenceRecord
}

type ConceptInventory struct {
	ConceptID         string   `json:"concept_id"`
	PrefLabel         string   `json:"pref_label"`
	Status            string   `json:"status"`
	Frequency         int      `json:"frequency"`
	EligibleFrequency int      `json:"eligible_frequency"`
	ExactAuthority    bool     `json:"exact_authority"`
	Languages         []string `json:"languages"`
	Surfaces          []string `json:"surfaces"`
}

type BilingualBacklogItem struct {
	ConceptID        string   `json:"concept_id"`
	PrefLabel        string   `json:"pref_label"`
	Frequency        int      `json:"frequency"`
	MissingLanguages []string `json:"missing_languages"`
}

type ContextSensitiveSurface struct {
	NormKey    string   `json:"norm_key"`
	Surfaces   []string `json:"surfaces"`
	ConceptIDs []string `json:"concept_ids"`
	Frequency  int      `json:"frequency"`
}

type CoverageReport struct {
	SchemaVersion            int                       `json:"schema_version"`
	Scope                    string                    `json:"scope"`
	Corpus                   []CorpusRef               `json:"corpus"`
	TargetSeedRelease        SeedRelease               `json:"target_seed_release"`
	TargetCoverage           float64                   `json:"target_coverage"`
	EligibleFrequency        int                       `json:"eligible_frequency"`
	CoveredFrequency         int                       `json:"covered_frequency"`
	Coverage                 float64                   `json:"coverage"`
	TargetMet                bool                      `json:"target_met"`
	RiskTerms                []string                  `json:"risk_terms"`
	UncoveredRiskTerms       []string                  `json:"uncovered_risk_terms"`
	RiskTermsMet             bool                      `json:"risk_terms_met"`
	Ready                    bool                      `json:"ready"`
	Approval                 string                    `json:"approval"`
	Approver                 string                    `json:"approver"`
	Inventory                []ConceptInventory        `json:"inventory"`
	UnresolvedBilingualPairs []BilingualBacklogItem    `json:"unresolved_bilingual_pairs"`
	ContextSensitiveSurfaces []ContextSensitiveSurface `json:"context_sensitive_surfaces"`
	HighFrequencyUncovered   []ConceptInventory        `json:"high_frequency_uncovered_concepts"`
}

func Measure(ctx context.Context, reader Reader, acceptance Acceptance) (CoverageReport, error) {
	if err := acceptance.Validate(); err != nil {
		return CoverageReport{}, err
	}
	data, err := reader.Load(ctx, CoverageQuery{Scope: acceptance.Scope, Corpus: acceptance.Corpus, TargetSeedRelease: acceptance.TargetSeedRelease})
	if err != nil {
		return CoverageReport{}, fmt.Errorf("load terminology coverage data: %w", err)
	}
	return BuildCoverage(acceptance, data)
}

func BuildCoverage(acceptance Acceptance, data CorpusData) (CoverageReport, error) {
	if err := acceptance.Validate(); err != nil {
		return CoverageReport{}, err
	}

	concepts := make(map[string]ConceptRecord)
	for _, concept := range data.Concepts {
		if concept.Scope == acceptance.Scope && (concept.Status == "active" || concept.Status == "provisional") {
			concepts[concept.ConceptID] = concept
		}
	}

	surfaceSets := make(map[string]map[string]struct{})
	languageSets := make(map[string]map[string]struct{})
	normConcepts := make(map[string]map[string]struct{})
	normSurfaces := make(map[string]map[string]struct{})
	termConcepts := make(map[string]map[string]struct{})
	for _, surface := range data.Surfaces {
		if surface.Scope != acceptance.Scope {
			continue
		}
		if _, ok := concepts[surface.ConceptID]; !ok {
			continue
		}
		addSet(surfaceSets, surface.ConceptID, surface.Surface)
		addSet(languageSets, surface.ConceptID, primaryLanguage(surface.Lang))
		addSet(normConcepts, surface.NormKey, surface.ConceptID)
		addSet(normSurfaces, surface.NormKey, surface.Surface)
		addSet(termConcepts, normalizedTerm(surface.Surface), surface.ConceptID)
	}
	for id, concept := range concepts {
		addSet(termConcepts, normalizedTerm(concept.PrefLabel), id)
	}

	selectedOccurrences := make([]OccurrenceRecord, 0, len(data.Occurrences))
	contextNorms := make(map[string]bool)
	for norm, ids := range normConcepts {
		if len(ids) > 1 {
			contextNorms[norm] = true
		}
	}
	for _, occurrence := range data.Occurrences {
		if occurrence.Scope != acceptance.Scope || !inCorpus(occurrence, acceptance.Corpus) {
			continue
		}
		selectedOccurrences = append(selectedOccurrences, occurrence)
		if occurrence.ResolutionStatus == "ambiguous" {
			contextNorms[occurrence.NormKey] = true
		}
	}

	totalFrequency := make(map[string]int)
	eligibleFrequency := make(map[string]int)
	normFrequency := make(map[string]int)
	for _, occurrence := range selectedOccurrences {
		normFrequency[occurrence.NormKey]++
		if _, ok := concepts[occurrence.ConceptID]; !ok {
			continue
		}
		totalFrequency[occurrence.ConceptID]++
		if !contextNorms[occurrence.NormKey] {
			eligibleFrequency[occurrence.ConceptID]++
		}
	}

	report := CoverageReport{
		SchemaVersion: AcceptanceSchemaVersion, Scope: acceptance.Scope,
		Corpus: acceptance.Corpus, TargetSeedRelease: acceptance.TargetSeedRelease,
		TargetCoverage: acceptance.TargetCoverage, RiskTerms: append([]string(nil), acceptance.RiskTerms...),
		Approver: acceptance.Approver, Approval: "operator_required",
	}
	for id, concept := range concepts {
		if totalFrequency[id] == 0 {
			continue
		}
		item := ConceptInventory{
			ConceptID: id, PrefLabel: concept.PrefLabel, Status: concept.Status,
			Frequency: totalFrequency[id], EligibleFrequency: eligibleFrequency[id],
			ExactAuthority: concept.ExactAuthority,
			Languages:      sortedSet(languageSets[id]), Surfaces: sortedSet(surfaceSets[id]),
		}
		report.Inventory = append(report.Inventory, item)
		report.EligibleFrequency += item.EligibleFrequency
		if item.ExactAuthority {
			report.CoveredFrequency += item.EligibleFrequency
		} else if item.EligibleFrequency > 0 {
			report.HighFrequencyUncovered = append(report.HighFrequencyUncovered, item)
		}
		missing := missingBilingualLanguages(languageSets[id])
		if len(missing) > 0 {
			report.UnresolvedBilingualPairs = append(report.UnresolvedBilingualPairs, BilingualBacklogItem{
				ConceptID: id, PrefLabel: concept.PrefLabel, Frequency: item.Frequency, MissingLanguages: missing,
			})
		}
	}

	for norm := range contextNorms {
		if normFrequency[norm] == 0 {
			continue
		}
		report.ContextSensitiveSurfaces = append(report.ContextSensitiveSurfaces, ContextSensitiveSurface{
			NormKey: norm, Surfaces: sortedSet(normSurfaces[norm]), ConceptIDs: sortedSet(normConcepts[norm]), Frequency: normFrequency[norm],
		})
	}
	sortInventory(report.Inventory)
	sortInventory(report.HighFrequencyUncovered)
	sort.Slice(report.UnresolvedBilingualPairs, func(i, j int) bool {
		if report.UnresolvedBilingualPairs[i].Frequency != report.UnresolvedBilingualPairs[j].Frequency {
			return report.UnresolvedBilingualPairs[i].Frequency > report.UnresolvedBilingualPairs[j].Frequency
		}
		return report.UnresolvedBilingualPairs[i].ConceptID < report.UnresolvedBilingualPairs[j].ConceptID
	})
	sort.Slice(report.ContextSensitiveSurfaces, func(i, j int) bool {
		if report.ContextSensitiveSurfaces[i].Frequency != report.ContextSensitiveSurfaces[j].Frequency {
			return report.ContextSensitiveSurfaces[i].Frequency > report.ContextSensitiveSurfaces[j].Frequency
		}
		return report.ContextSensitiveSurfaces[i].NormKey < report.ContextSensitiveSurfaces[j].NormKey
	})

	if report.EligibleFrequency > 0 {
		report.Coverage = float64(report.CoveredFrequency) / float64(report.EligibleFrequency)
	}
	report.TargetMet = report.Coverage >= acceptance.TargetCoverage
	for _, risk := range acceptance.RiskTerms {
		ids := termConcepts[normalizedTerm(risk)]
		covered := len(ids) > 0
		for id := range ids {
			if !concepts[id].ExactAuthority || totalFrequency[id] == 0 {
				covered = false
				break
			}
		}
		if !covered {
			report.UncoveredRiskTerms = append(report.UncoveredRiskTerms, risk)
		}
	}
	sort.Strings(report.UncoveredRiskTerms)
	report.RiskTermsMet = len(report.UncoveredRiskTerms) == 0
	report.Ready = report.TargetMet && report.RiskTermsMet
	return report, nil
}

func sortInventory(items []ConceptInventory) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Frequency != items[j].Frequency {
			return items[i].Frequency > items[j].Frequency
		}
		if items[i].PrefLabel != items[j].PrefLabel {
			return items[i].PrefLabel < items[j].PrefLabel
		}
		return items[i].ConceptID < items[j].ConceptID
	})
}

func addSet(sets map[string]map[string]struct{}, key, value string) {
	if value == "" {
		return
	}
	if sets[key] == nil {
		sets[key] = make(map[string]struct{})
	}
	sets[key][value] = struct{}{}
}

func sortedSet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func primaryLanguage(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(lang, "-_"); i >= 0 {
		lang = lang[:i]
	}
	return lang
}

func missingBilingualLanguages(languages map[string]struct{}) []string {
	var missing []string
	for _, lang := range []string{"en", "zh"} {
		if _, ok := languages[lang]; !ok {
			missing = append(missing, lang)
		}
	}
	return missing
}

func normalizedTerm(term string) string {
	return strings.ToLower(strings.TrimSpace(term))
}

func inCorpus(occurrence OccurrenceRecord, corpus []CorpusRef) bool {
	for _, ref := range corpus {
		if occurrence.ArtifactType != ref.ArtifactType {
			continue
		}
		if len(ref.ArtifactIDs) == 0 {
			return true
		}
		for _, id := range ref.ArtifactIDs {
			if occurrence.ArtifactID == id {
				return true
			}
		}
	}
	return false
}

type SQLQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type SQLReader struct {
	DB SQLQueryer
}

func (r SQLReader) Load(ctx context.Context, query CoverageQuery) (CorpusData, error) {
	var data CorpusData
	rows, err := r.DB.QueryContext(ctx, `
		SELECT c.concept_id, c.pref_label, c.scope, c.status,
		       EXISTS (
		           SELECT 1
		           FROM kb.keyword_external_ids x
		           JOIN kb.keyword_sources src
		             ON src.source = x.source AND src.release = x.release
		           WHERE x.concept_id = c.concept_id AND x.source = $2 AND x.release = $3
		       ) AS exact_authority
		FROM kb.keyword_concepts c
		WHERE c.scope = $1 AND c.status IN ('active', 'provisional')
		ORDER BY c.concept_id`, query.Scope, query.TargetSeedRelease.Source, query.TargetSeedRelease.Release)
	if err != nil {
		return CorpusData{}, fmt.Errorf("query concepts: %w", err)
	}
	for rows.Next() {
		var row ConceptRecord
		if err := rows.Scan(&row.ConceptID, &row.PrefLabel, &row.Scope, &row.Status, &row.ExactAuthority); err != nil {
			rows.Close()
			return CorpusData{}, fmt.Errorf("scan concept: %w", err)
		}
		data.Concepts = append(data.Concepts, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return CorpusData{}, fmt.Errorf("iterate concepts: %w", err)
	}
	rows.Close()

	rows, err = r.DB.QueryContext(ctx, `
		SELECT s.concept_id, s.surface, s.norm_key, s.lang, s.scope
		FROM kb.keyword_surfaces s
		JOIN kb.keyword_concepts c ON c.concept_id = s.concept_id
		WHERE s.scope = $1 AND c.scope = $1 AND c.status IN ('active', 'provisional')
		ORDER BY s.concept_id, s.surface_id`, query.Scope)
	if err != nil {
		return CorpusData{}, fmt.Errorf("query surfaces: %w", err)
	}
	for rows.Next() {
		var row SurfaceRecord
		if err := rows.Scan(&row.ConceptID, &row.Surface, &row.NormKey, &row.Lang, &row.Scope); err != nil {
			rows.Close()
			return CorpusData{}, fmt.Errorf("scan surface: %w", err)
		}
		data.Surfaces = append(data.Surfaces, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return CorpusData{}, fmt.Errorf("iterate surfaces: %w", err)
	}
	rows.Close()

	rows, err = r.DB.QueryContext(ctx, `
		SELECT occurrence_id, artifact_type, artifact_id, COALESCE(concept_id, ''), norm_key, scope, resolution_status
		FROM kb.keyword_occurrences
		WHERE scope = $1
		ORDER BY occurrence_id`, query.Scope)
	if err != nil {
		return CorpusData{}, fmt.Errorf("query occurrences: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var row OccurrenceRecord
		if err := rows.Scan(&row.OccurrenceID, &row.ArtifactType, &row.ArtifactID, &row.ConceptID, &row.NormKey, &row.Scope, &row.ResolutionStatus); err != nil {
			return CorpusData{}, fmt.Errorf("scan occurrence: %w", err)
		}
		data.Occurrences = append(data.Occurrences, row)
	}
	if err := rows.Err(); err != nil {
		return CorpusData{}, fmt.Errorf("iterate occurrences: %w", err)
	}
	return data, nil
}
