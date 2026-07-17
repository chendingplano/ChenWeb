## 1. Struct and envelope changes (`server/api/doc-reviews/models.go`)

- [x] 1.1 Add `RunID int64 \`json:"run_id,omitempty"\`` to `FindingItem` (models.go:70-99, near `ModelName`)
- [x] 1.2 Add `RunID int64` to `FindingMetadataEnvelope` (models.go:146-155, near `ModelName`)
- [x] 1.3 Add `"run_id": true` to `findingMetadataReservedKeys` (models.go:126-139)
- [x] 1.4 In `FindingMetadataEnvelope.MarshalJSON` (models.go:157-193), add `if e.RunID != 0 { m["run_id"] = e.RunID }`
- [x] 1.5 In `FindingMetadataEnvelope.UnmarshalJSON` (models.go:199-259), add a case reading `raw["run_id"]` into `e.RunID`, alongside the existing `model_name` case
- [x] 1.6 In `applyFindingMetadata` (models.go:264-277), add `f.RunID = metadata.RunID`

## 2. Populate at the single write path (`server/api/doc-reviews/review-document.go`)

- [x] 2.1 In `ReviewFindingsSQLStore.SaveFindings` (review-document.go:230-291), in the loop over `preparedFindings` that marshals `prepared.Metadata`, set `prepared.Metadata.RunID = runID` before `json.Marshal(prepared.Metadata)`

## 3. Verification

- [x] 3.1 Run `go build ./...` and `go test ./...` in `ChenWeb/server/api/doc-reviews` (and workspace-wide `go vet ./...` per repo convention)
- [ ] 3.2 Run a live doc review (or the closest existing integration/smoke test); confirm `metadata.run_id` is populated on resulting `kb.doc_review_findings` rows and `run_id` appears on the API `FindingItem` response, matching the row's real `run_id` column

## 4. Documentation

- [x] 4.1 Add a change-log entry to `KnowledgeStore/doc-repo/adrs/202607/2026070201-adr-document-review-changes.md` noting that findings now carry `metadata.run_id`
