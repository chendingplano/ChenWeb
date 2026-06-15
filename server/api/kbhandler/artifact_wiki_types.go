package kbhandler

import "encoding/json"

type artifactWikiGeneratedMeta struct {
	Model         string `json:"model"`
	Lang          string `json:"lang"`
	SchemaVersion int    `json:"schema_version"`
	SourceHash    string `json:"source_hash"`
}

type artifactWikiResponse struct {
	Status         bool                     `json:"status"`
	Fresh          bool                     `json:"fresh"`
	ArtifactType   string                   `json:"artifact_type"`
	ArtifactID     string                   `json:"artifact_id"`
	Article        json.RawMessage          `json:"article,omitempty"`
	Record         json.RawMessage          `json:"record"`
	SourceDocument json.RawMessage          `json:"source_document,omitempty"`
	Generated      artifactWikiGeneratedMeta `json:"generated"`
}

type artifactWikiMetricPayload struct {
	RecordID       int64
	ArtifactID     string
	Record         json.RawMessage
	SourceDocument json.RawMessage
	Generated      artifactWikiGeneratedMeta
}
