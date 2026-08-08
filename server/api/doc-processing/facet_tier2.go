package docprocessing

import (
	"context"
	"regexp"
	"strings"
)

// Tier-2 facets (spec ADR 2026072901 S3.5 DR4, S16.1 "Facet tiers 1-2"):
// derived from extract_doc_metadata's already-extracted output (doc_no,
// publish_date) -- already paid for, no new LLM call. Reads through the
// same DocMetadataStore interface every other caller in this package uses
// (DocumentNumber and PublishDate, extract-doc-metadata-store.go) rather
// than a raw *sql.DB, so this fits ExtractDocMetadataProcessor's existing
// store-interface wiring without inventing a parallel access path.
//
// DR4's original tier-2 list ("issuer, edition, publish date, authority
// hints from title/doc-no") doesn't map 1:1 onto what extract_doc_metadata
// actually extracts today: there is no structured Issuer or Edition field
// (only a free-form "other metadata only" bag with no fixed schema), so
// those are not implemented here -- they would need a prompt/schema change
// to extract_doc_metadata itself, which is new extraction work, not facet
// wiring. What is implemented: publish_date (direct passthrough) and
// authority_hint, which folds "issuer hint" and "doc-number pattern" into
// one signal, both being the same underlying computation (a regex over the
// document number's prefix) approached from two angles in the original plan.
// Prefix-only matching, deliberately without a trailing \b: a document
// number's prefix commonly abuts its digits with no separator (e.g.
// "GB50325-2010"), and since digits are \w characters, "B" -> "5" is not a
// word-boundary transition -- a \b there would silently reject exactly the
// unspaced form real document numbers use.
var authorityHintPatterns = []struct {
	hint    string
	pattern *regexp.Regexp
}{
	{"gb", regexp.MustCompile(`(?i)^\s*(GB[/\-]?T?|HG|JB|QB|SN|YY)`)},
	{"iso", regexp.MustCompile(`(?i)^\s*ISO`)},
	{"iec", regexp.MustCompile(`(?i)^\s*IEC`)},
	{"ansi", regexp.MustCompile(`(?i)^\s*ANSI`)},
	{"astm", regexp.MustCompile(`(?i)^\s*ASTM`)},
}

// ComputeTier2Facets reads recordID's already-extracted document number and
// publish date via metaStore and returns its metadata-derived facet
// observations. Returns (nil, nil) if the record has neither field
// populated yet (the caller should treat this as "not ready", not an
// error) -- extract_doc_metadata may not have run yet, or found nothing.
func ComputeTier2Facets(ctx context.Context, metaStore DocMetadataStore, recordID int64) ([]FacetObservation, error) {
	rec, err := metaStore.GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, err
	}
	return tier2FacetsFromSource(recordID, rec.DocumentNumber, rec.PublishDate), nil
}

func tier2FacetsFromSource(recordID int64, docNo, publishDate string) []FacetObservation {
	var obs []FacetObservation
	add := func(path string, value any) {
		obs = append(obs, FacetObservation{
			RecordID:   recordID,
			Path:       path,
			Value:      value,
			Method:     FacetMethodMetadata,
			Confidence: deterministicConfidence(),
		})
	}

	if publishDate != "" {
		add("document.publish_date", publishDate)
	}
	if hint := authorityHintFromDocNo(docNo); hint != "" {
		add("document.authority_hint", hint)
	}
	return obs
}

// authorityHintFromDocNo classifies a document number's issuing-body prefix.
// Returns "" (no observation) when docNo is empty -- absence of a document
// number is a different fact (document.has_document_number, already wired)
// from "has a number but it matched no known pattern" ("unknown").
func authorityHintFromDocNo(docNo string) string {
	docNo = strings.TrimSpace(docNo)
	if docNo == "" {
		return ""
	}
	for _, p := range authorityHintPatterns {
		if p.pattern.MatchString(docNo) {
			return p.hint
		}
	}
	return "unknown"
}
