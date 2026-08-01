package assertions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ProvisionNormalizer is the seam-5 instantiation for the provision artifact
// family. Modality (required/prohibited/permitted) is not yet a column on
// kb.provisions -- extract_provisions' applicability/authority/effective-
// interval columns are ADR P3-P4 "changed processor" work, not yet built --
// so this normalizer extracts modality from the provision text itself
// (mirroring the metric normalizer's text-parsing approach). A provision
// whose modality cannot be determined still produces a candidate with
// modality='unparsed', never a dropped artifact.
type ProvisionNormalizer struct{}

// FamilyName implements Normalizer.
func (ProvisionNormalizer) FamilyName() string { return "provision" }

// init registers the provision normalizer with the default registry (DR11
// seam 5).
func init() {
	if err := RegisterNormalizer(ProvisionNormalizer{}); err != nil {
		panic(err)
	}
}

type provisionRow struct {
	ID               int64
	ProvID           string
	ProvisionType    sql.NullString
	Provision        sql.NullString
	ProvisionSubject sql.NullString
	ProvDesc         sql.NullString
	Confidence       sql.NullFloat64
	SourceLineSpans  json.RawMessage
	SubjectObjectID  sql.NullString
}

// Normalize implements Normalizer.
func (n ProvisionNormalizer) Normalize(ctx context.Context, db *sql.DB, inputRecordID int64) (NormalizeReport, error) {
	report := NormalizeReport{ArtifactFamily: n.FamilyName()}
	if db == nil {
		return report, fmt.Errorf("db is nil")
	}

	const stmt = `
SELECT p.id, p.prov_id, p.provision_type, p.provision, p.provision_subject,
       p.prov_desc, p.confidence, p.source_line_spans, ao.object_id
FROM kb.provisions p
LEFT JOIN kb.artifact_objects ao
  ON ao.artifact_type = 'provision' AND ao.artifact_id = p.prov_id AND ao.input_record_id = p.input_record_id
WHERE p.input_record_id = $1`
	rows, err := db.QueryContext(ctx, stmt, inputRecordID)
	if err != nil {
		return report, err
	}
	defer rows.Close()

	dcStore := DecisionCandidateStore{DB: db}
	for rows.Next() {
		var r provisionRow
		if err := rows.Scan(&r.ID, &r.ProvID, &r.ProvisionType, &r.Provision, &r.ProvisionSubject,
			&r.ProvDesc, &r.Confidence, &r.SourceLineSpans, &r.SubjectObjectID); err != nil {
			return report, err
		}
		report.Examined++

		provID := strings.TrimSpace(r.ProvID)
		if provID == "" {
			report.Skipped++
			continue
		}
		text := strings.TrimSpace(r.Provision.String)
		if text == "" {
			text = strings.TrimSpace(r.ProvDesc.String)
		}
		modality := parseProvisionModality(text)

		payload := map[string]any{
			"prov_id":        provID,
			"provision_type": r.ProvisionType.String,
			"subject":        r.ProvisionSubject.String,
			"raw_text":       text,
			"modality":       modality,
			"assertion_kind": provisionAssertionKind(modality),
		}
		if r.SubjectObjectID.Valid && r.SubjectObjectID.String != "" {
			payload["subject_object_id"] = r.SubjectObjectID.String
		}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return report, err
		}

		candidate := DecisionCandidate{
			LogicalIdentityKey: fmt.Sprintf("provision:%d:%s", inputRecordID, provID),
			CandidateKind:      "assertion",
			ProposedPayload:    payloadJSON,
			Method:             "explicit_structured",
			SourceArtifactType: "provision",
			SourceArtifactID:   provID,
			SourceLineSpans:    r.SourceLineSpans,
		}
		if r.Confidence.Valid {
			v := r.Confidence.Float64
			candidate.Confidence = &v
		}

		result, err := dcStore.Propose(ctx, candidate)
		if err != nil {
			return report, fmt.Errorf("propose candidate for provision %s: %w", provID, err)
		}
		if result.Reused {
			report.Reused++
		} else {
			report.Proposed++
		}
	}
	return report, rows.Err()
}

// The pilot corpus (ADR OD1: 呼吸机/医疗器械) is predominantly Chinese-language
// standard text, so the modality vocabulary covers both languages: 不应/不得/
// 禁止/严禁 ("shall not"/"must not"/"prohibited"), 应/须/必须 ("shall"/"must"),
// 宜 ("should"), 可 ("may") are the forms observed in the gold corpus.
var (
	reProhibited  = regexp.MustCompile(`(?i)(shall not|must not|prohibited|not permitted|may not|不应|不得|禁止|严禁)`)
	reRequired    = regexp.MustCompile(`(?i)(shall|must|is required to|are required to|应|须|必须)`)
	rePermitted   = regexp.MustCompile(`(?i)(may|is permitted to|are permitted to|is allowed to|可)`)
	reRecommended = regexp.MustCompile(`(?i)(should|is recommended to|宜)`)
)

// parseProvisionModality returns 'prohibited' | 'required' | 'permitted' |
// 'recommended' | 'unparsed'. Order matters: negated forms ("shall not",
// "不应") must be checked before their unnegated prefix ("shall", "应")
// matches -- 不应 contains 应, so the prohibited check must run first.
func parseProvisionModality(text string) string {
	switch {
	case reProhibited.MatchString(text):
		return "prohibited"
	case reRequired.MatchString(text):
		return "required"
	case reRecommended.MatchString(text):
		return "recommended"
	case rePermitted.MatchString(text):
		return "permitted"
	default:
		return "unparsed"
	}
}

func provisionAssertionKind(modality string) string {
	switch modality {
	case "prohibited":
		return "prohibited"
	case "required":
		return "required"
	case "permitted", "recommended":
		return "permitted"
	default:
		return "unparsed"
	}
}
