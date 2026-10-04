package agentservicehandler

import (
	"bufio"
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/kbhandler"
	"github.com/chendingplano/deepdoc/server/api/pathutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

const (
	MaxToolQueryBytes    = 512
	MaxToolResults       = 20
	MaxToolRanges        = 4
	MaxToolLines         = 120
	MaxToolResponseBytes = 128 * 1024
	HeaderRunID          = "X-ChenWeb-Run-ID"
	HeaderRunCapability  = "X-ChenWeb-Run-Capability"
	// HeaderToolCallID names the gateway's tool call, under which ChenWeb
	// stores the call's arguments and result.
	HeaderToolCallID = "X-ChenWeb-Tool-Call-ID"
)

var (
	ErrKnowledgeAccessDenied = errors.New("knowledge access denied")
	ErrInvalidToolInput      = errors.New("invalid knowledge tool input")
	ErrToolResponseTooLarge  = errors.New("knowledge tool response too large")
)

type KnowledgeAccessRequest struct {
	UserID, ProfileSlug, ProfileVersion, ToolName string
	KnowledgeStoreID, DocumentGroup, DocumentID   string
	AllowedKnowledgeStoreNames                    []string
}

type InternalToolHandler struct {
	gatewaySecret string
	signer        *CapabilitySigner
	service       *KnowledgeToolService
	// results, profiles and documents are optional: without them tool calls
	// are not stored and get_saved_tool_result is unavailable.
	results   ToolResultStore
	profiles  *ProfileRegistry
	documents DocumentAccessChecker
	now       func() time.Time
}

// ToolResultStore keeps each tool call's arguments and full result.
type ToolResultStore interface {
	SaveToolResult(context.Context, string, ToolResultRecord) error
	LoadSavedToolResult(context.Context, string, string, string) (ToolResultRecord, string, error)
}

// DocumentAccessChecker checks only current access to a document.
type DocumentAccessChecker interface {
	CheckDocumentAccess(context.Context, string, []string, []string, string) error
}

func NewInternalToolHandler(gatewaySecret string, signer *CapabilitySigner, service *KnowledgeToolService) *InternalToolHandler {
	return &InternalToolHandler{gatewaySecret: gatewaySecret, signer: signer, service: service, now: time.Now}
}

// WithToolResults stores tool calls and enables get_saved_tool_result.
func (h *InternalToolHandler) WithToolResults(results ToolResultStore, profiles *ProfileRegistry, documents DocumentAccessChecker) *InternalToolHandler {
	h.results, h.profiles, h.documents = results, profiles, documents
	return h
}

func RegisterInternalToolRoutes(e *echo.Echo, handler *InternalToolHandler) {
	group := e.Group("/api/internal/agent-tools")
	for _, tool := range knowledgeToolNames {
		tool := tool
		group.POST("/"+tool, func(c echo.Context) error { return handler.execute(c, tool) })
	}
	group.POST("/"+savedToolResultTool, handler.executeSavedResult)
}

// authorize checks the gateway secret and the run capability for tool.
func (h *InternalToolHandler) authorize(c echo.Context, tool string) (RunCapabilityClaims, int, string) {
	authorization := c.Request().Header.Get(echo.HeaderAuthorization)
	if !strings.HasPrefix(authorization, "Bearer ") || strings.Count(authorization, " ") != 1 ||
		!constantTimeStringEqual(strings.TrimPrefix(authorization, "Bearer "), h.gatewaySecret) {
		return RunCapabilityClaims{}, http.StatusUnauthorized, "unauthorized"
	}
	claims, err := h.signer.Verify(c.Request().Header.Get(HeaderRunCapability), c.Request().Header.Get(HeaderRunID), tool)
	if err != nil {
		return RunCapabilityClaims{}, http.StatusUnauthorized, "invalid run authorization"
	}
	return claims, 0, ""
}

// respond returns body to Pi and stores the call under the gateway's
// tool-call ID. A storage failure is logged; Pi still gets its result.
func (h *InternalToolHandler) respond(c echo.Context, claims RunCapabilityClaims, tool string, arguments any, status int, body []byte, documentIDs []string) error {
	toolCallID := strings.TrimSpace(c.Request().Header.Get(HeaderToolCallID))
	if h.results != nil && toolCallID != "" && len(toolCallID) <= 128 {
		encodedArguments, _ := json.Marshal(arguments)
		record := ToolResultRecord{AttemptID: claims.RunID, ToolCallID: toolCallID, ToolName: tool, Arguments: encodedArguments,
			Result: string(body), IsError: status != http.StatusOK, DocumentIDs: documentIDs}
		if err := h.results.SaveToolResult(c.Request().Context(), claims.UserID, record); err != nil {
			loggerutil.CreateDefaultLogger("20261004-731").Warn("store agent tool result failed", "run_id", claims.RunID, "tool", tool, "error", err)
		}
	}
	return c.JSONBlob(status, body)
}

func errorBody(message string) []byte {
	body, _ := json.Marshal(map[string]string{"error": message})
	return body
}

func (h *InternalToolHandler) execute(c echo.Context, tool string) error {
	if h == nil || h.signer == nil || h.service == nil || h.gatewaySecret == "" {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "internal knowledge tools unavailable"})
	}
	claims, status, message := h.authorize(c, tool)
	if status != 0 {
		return c.JSON(status, map[string]string{"error": message})
	}
	var input ToolInput
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 16*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return h.respond(c, claims, tool, struct{}{}, http.StatusBadRequest, errorBody("invalid tool request"), nil)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return h.respond(c, claims, tool, input, http.StatusBadRequest, errorBody("invalid tool request"), nil)
	}
	documentIDs := appendDocumentID(nil, input.DocumentID)
	out, err := h.service.Execute(c.Request().Context(), claims, tool, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidToolInput):
			return h.respond(c, claims, tool, input, http.StatusBadRequest, errorBody(err.Error()), documentIDs)
		case errors.Is(err, ErrKnowledgeAccessDenied):
			return h.respond(c, claims, tool, input, http.StatusForbidden, errorBody(err.Error()), documentIDs)
		case errors.Is(err, ErrToolResponseTooLarge):
			return h.respond(c, claims, tool, input, http.StatusRequestEntityTooLarge, errorBody(err.Error()), documentIDs)
		default:
			return h.respond(c, claims, tool, input, http.StatusInternalServerError, errorBody("knowledge tool failed"), documentIDs)
		}
	}
	body, err := json.Marshal(out)
	if err != nil {
		return h.respond(c, claims, tool, input, http.StatusInternalServerError, errorBody("knowledge tool failed"), documentIDs)
	}
	for _, item := range out.Items {
		documentIDs = appendDocumentID(documentIDs, item.DocumentID)
	}
	return h.respond(c, claims, tool, input, http.StatusOK, body, documentIDs)
}

// executeSavedResult returns the stored result of an earlier tool call in the
// same conversation, under the snapshot rule: a result older than the
// snapshot period is withheld if it references a document the user can no
// longer access.
func (h *InternalToolHandler) executeSavedResult(c echo.Context) error {
	if h == nil || h.signer == nil || h.gatewaySecret == "" || h.results == nil || h.profiles == nil || h.documents == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "saved tool results unavailable"})
	}
	claims, status, message := h.authorize(c, savedToolResultTool)
	if status != 0 {
		return c.JSON(status, map[string]string{"error": message})
	}
	var input struct {
		ToolCallID string `json:"tool_call_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.ToolCallID) == "" || len(input.ToolCallID) > 128 {
		return h.respond(c, claims, savedToolResultTool, input, http.StatusBadRequest, errorBody("tool_call_id is required"), nil)
	}
	ctx := c.Request().Context()
	notFound := errorBody("saved tool result not found")
	record, _, err := h.results.LoadSavedToolResult(ctx, claims.UserID, claims.RunID, input.ToolCallID)
	if err != nil {
		return h.respond(c, claims, savedToolResultTool, input, http.StatusNotFound, notFound, nil)
	}
	if h.now().Sub(record.CreatedAt) > h.profiles.SnapshotWindow() {
		profile, err := h.profiles.ResolveVersion(claims.ProfileSlug, claims.ProfileVersion, claims.UserID)
		if err != nil {
			return h.respond(c, claims, savedToolResultTool, input, http.StatusNotFound, notFound, nil)
		}
		for _, documentID := range record.DocumentIDs {
			if h.documents.CheckDocumentAccess(ctx, claims.UserID, profile.AllowedKnowledgeStores, profile.AllowedDocumentGroups, documentID) != nil {
				return h.respond(c, claims, savedToolResultTool, input, http.StatusNotFound, notFound, nil)
			}
		}
	}
	return h.respond(c, claims, savedToolResultTool, input, http.StatusOK, []byte(record.Result), record.DocumentIDs)
}

func appendDocumentID(ids []string, id string) []string {
	if id == "" || stringIn(ids, id) {
		return ids
	}
	return append(ids, id)
}

func constantTimeStringEqual(a, b string) bool {
	return len(a) == len(b) && hmacEqual([]byte(a), []byte(b))
}

func hmacEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

type KnowledgeAccessChecker interface {
	CheckKnowledgeAccess(context.Context, KnowledgeAccessRequest) error
}

type ProfileAccessChecker struct {
	Registry *ProfileRegistry
	Next     KnowledgeAccessChecker
}

func (p ProfileAccessChecker) CheckKnowledgeAccess(ctx context.Context, req KnowledgeAccessRequest) error {
	if p.Registry == nil || p.Next == nil {
		return ErrKnowledgeAccessDenied
	}
	profile, err := p.Registry.ResolveVersion(req.ProfileSlug, req.ProfileVersion, req.UserID)
	if err != nil || !stringIn(profile.AllowedTools, req.ToolName) ||
		(req.DocumentGroup != "" && len(profile.AllowedDocumentGroups) > 0 && !stringIn(profile.AllowedDocumentGroups, req.DocumentGroup)) {
		return ErrKnowledgeAccessDenied
	}
	req.AllowedKnowledgeStoreNames = append([]string(nil), profile.AllowedKnowledgeStores...)
	if err := p.Next.CheckKnowledgeAccess(ctx, req); err != nil {
		return ErrKnowledgeAccessDenied
	}
	return nil
}

// ActiveKnowledgeAccessChecker is the fail-closed default. The capability issuer
// establishes user scope; this checker revalidates that the scoped resource is
// still active on every call. Deployments with finer ACLs replace this interface.
type ActiveKnowledgeAccessChecker struct{ DB *sql.DB }

func (a ActiveKnowledgeAccessChecker) CheckKnowledgeAccess(ctx context.Context, req KnowledgeAccessRequest) error {
	if a.DB == nil || strings.TrimSpace(req.UserID) == "" {
		return ErrKnowledgeAccessDenied
	}
	var allowed bool
	err := a.DB.QueryRowContext(ctx, `SELECT EXISTS (
SELECT 1 FROM kb.agentic_knowledge_grants g
JOIN kb.knowledge_store ks ON ks.id=g.knowledge_store_id
LEFT JOIN kb.inputs i ON i.ks_store_id=ks.id AND ($3='' OR i.id::text=$3)
WHERE g.user_id=$1 AND ks.id::text=$2 AND ks.status='active'
  AND g.active AND (g.expires_at IS NULL OR g.expires_at > now())
  AND ks.ks_name=ANY($5)
  AND (($3='' AND g.document_id IS NULL) OR
       ($3<>'' AND i.id IS NOT NULL AND (g.document_id IS NULL OR g.document_id=i.id)))
  AND ($4='' OR i.type=$4))`, req.UserID, req.KnowledgeStoreID, req.DocumentID, req.DocumentGroup, pq.Array(req.AllowedKnowledgeStoreNames)).Scan(&allowed)
	if err != nil || !allowed {
		return ErrKnowledgeAccessDenied
	}
	return nil
}

type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type ToolInput struct {
	Query            string      `json:"query,omitempty"`
	KnowledgeStoreID string      `json:"knowledge_store_id"`
	DocumentGroup    string      `json:"document_group,omitempty"`
	DocumentID       string      `json:"document_id,omitempty"`
	ArtifactID       string      `json:"artifact_id,omitempty"`
	ArtifactType     string      `json:"artifact_type,omitempty"`
	Ranges           []LineRange `json:"ranges,omitempty"`
	Limit            int         `json:"limit,omitempty"`
}

type EvidenceItem struct {
	KnowledgeStoreID   string          `json:"knowledge_store_id"`
	DocumentGroup      string          `json:"document_group,omitempty"`
	DocumentID         string          `json:"document_id"`
	ArtifactID         string          `json:"artifact_id,omitempty"`
	ArtifactType       string          `json:"artifact_type,omitempty"`
	SourceTitle        string          `json:"source_title,omitempty"`
	SourceVersion      string          `json:"source_version,omitempty"`
	SourceFingerprint  string          `json:"source_fingerprint,omitempty"`
	Page               int             `json:"page,omitempty"`
	LineStart          int             `json:"line_start,omitempty"`
	LineEnd            int             `json:"line_end,omitempty"`
	ValidationStatus   string          `json:"validation_status,omitempty"`
	RelevanceScore     *float64        `json:"relevance_score,omitempty"`
	Relationship       string          `json:"relationship,omitempty"`
	RelationshipReason string          `json:"relationship_reason,omitempty"`
	UntrustedEvidence  bool            `json:"untrusted_evidence"`
	Content            json.RawMessage `json:"content,omitempty"`
}

type ToolOutput struct {
	Items []EvidenceItem `json:"items"`
}

type KnowledgeToolBackend interface {
	Execute(context.Context, string, ToolInput) (ToolOutput, error)
}

type SQLKnowledgeToolBackend struct{ DB *sql.DB }

func NewSQLKnowledgeToolBackend(db *sql.DB) *SQLKnowledgeToolBackend {
	return &SQLKnowledgeToolBackend{DB: db}
}

func (b *SQLKnowledgeToolBackend) Execute(ctx context.Context, tool string, input ToolInput) (ToolOutput, error) {
	if b == nil || b.DB == nil {
		return ToolOutput{}, errors.New("knowledge database unavailable")
	}
	switch tool {
	case "search_knowledge":
		return b.search(ctx, input)
	case "read_source_passages":
		return b.readPassages(ctx, input)
	case "get_artifact_details":
		return b.artifactDetails(ctx, input)
	case "get_document_context":
		return b.documentContext(ctx, input)
	case "find_related_knowledge":
		return b.related(ctx, input)
	default:
		return ToolOutput{}, ErrInvalidToolInput
	}
}

const artifactSelect = `
SELECT ks.id::text, COALESCE(i.type, ''), i.id::text, sa.artifact_id,
       sa.artifact_type, COALESCE(sa.source_title, i.title, ''),
	       COALESCE(sa.source_line_spans, '[]'::jsonb),
	       COALESCE(NULLIF(sa.semantic_payload->>'validation_status', ''), 'unreviewed'),
	       i.modify_time::text,
	       COALESCE(NULLIF(i.md5, ''), 'input:' || i.id::text || ':' || EXTRACT(EPOCH FROM i.modify_time)::bigint::text),
	       COALESCE(sa.primary_label, ''), COALESCE(sa.secondary_label, ''),
	       LEFT(COALESCE(NULLIF(sa.snippet_basis, ''), sa.search_document, ''), 8000),
	       LEFT(COALESCE(sa.semantic_payload::text, '{}'), 16000)
FROM kb.search_artifacts sa
JOIN kb.inputs i ON i.id = sa.input_record_id
JOIN kb.knowledge_store ks ON ks.id = i.ks_store_id`

func (b *SQLKnowledgeToolBackend) search(ctx context.Context, in ToolInput) (ToolOutput, error) {
	hits, err := kbhandler.SearchAgentKnowledge(ctx, b.DB, in.Query, in.KnowledgeStoreID, in.DocumentGroup, in.DocumentID, in.ArtifactType, in.Limit)
	if err != nil {
		return ToolOutput{}, err
	}
	out := ToolOutput{Items: make([]EvidenceItem, 0, len(hits))}
	for _, hit := range hits {
		page, start, end := firstLocation(hit.SourceLineSpans)
		score := hit.Score
		content, _ := json.Marshal(map[string]string{"primary_label": hit.PrimaryLabel, "secondary_label": hit.SecondaryLabel, "snippet": hit.Snippet})
		out.Items = append(out.Items, EvidenceItem{
			KnowledgeStoreID: hit.KnowledgeStoreID, DocumentGroup: hit.DocumentGroup, DocumentID: hit.DocumentID,
			ArtifactID: hit.ArtifactID, ArtifactType: hit.ArtifactType, SourceTitle: hit.SourceTitle,
			SourceVersion: hit.SourceVersion, SourceFingerprint: hit.SourceFingerprint,
			Page: page, LineStart: start, LineEnd: end, ValidationStatus: hit.ValidationStatus,
			RelevanceScore: &score, UntrustedEvidence: true, Content: content,
		})
	}
	return out, nil
}

func (b *SQLKnowledgeToolBackend) artifactDetails(ctx context.Context, in ToolInput) (ToolOutput, error) {
	rows, err := b.DB.QueryContext(ctx, artifactSelect+`
WHERE ks.status = 'active' AND ks.id::text = $1 AND sa.artifact_id = $2
  AND ($3 = '' OR sa.artifact_type = $3) AND ($4 = '' OR i.id::text = $4)
  AND ($5 = '' OR i.type = $5)
ORDER BY sa.artifact_type LIMIT 1`, in.KnowledgeStoreID, in.ArtifactID, in.ArtifactType, in.DocumentID, in.DocumentGroup)
	if err != nil {
		return ToolOutput{}, err
	}
	defer rows.Close()
	return scanEvidenceRows(rows)
}

func (b *SQLKnowledgeToolBackend) related(ctx context.Context, in ToolInput) (ToolOutput, error) {
	query := artifactSelect
	args := []any{in.KnowledgeStoreID, in.DocumentID, in.DocumentGroup, in.Limit}
	relationship, reason := "same_document", "This artifact was extracted from the same source document."
	if in.ArtifactID != "" {
		query += `
JOIN kb.search_artifacts anchor ON anchor.artifact_id=$4 AND ($5='' OR anchor.artifact_type=$5)
JOIN kb.inputs anchor_input ON anchor_input.id=anchor.input_record_id
WHERE ks.status='active' AND ks.id::text=$1 AND ($2='' OR i.id::text=$2)
  AND anchor_input.ks_store_id=ks.id AND ($2='' OR anchor_input.id::text=$2)
  AND ($3='' OR (i.type=$3 AND anchor_input.type=$3))
  AND NOT (sa.artifact_id=anchor.artifact_id AND sa.artifact_type=anchor.artifact_type)
  AND (sa.input_record_id=anchor.input_record_id OR sa.keywords && anchor.keywords)
ORDER BY (sa.keywords && anchor.keywords) DESC, (sa.input_record_id=anchor.input_record_id) DESC, sa.artifact_id
LIMIT $6`
		args = []any{in.KnowledgeStoreID, in.DocumentID, in.DocumentGroup, in.ArtifactID, in.ArtifactType, in.Limit}
		relationship, reason = "shared_source_or_keywords", "This artifact shares a source document or indexed keywords with the starting artifact."
	} else {
		query += `
WHERE ks.status='active' AND ks.id::text=$1 AND i.id::text=$2 AND ($3='' OR i.type=$3)
ORDER BY sa.artifact_type, sa.artifact_id LIMIT $4`
	}
	rows, err := b.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return ToolOutput{}, err
	}
	defer rows.Close()
	out, err := scanEvidenceRows(rows)
	for i := range out.Items {
		out.Items[i].Relationship = relationship
		out.Items[i].RelationshipReason = reason
	}
	return out, err
}

func scanEvidenceRows(rows *sql.Rows) (ToolOutput, error) {
	out := ToolOutput{Items: []EvidenceItem{}}
	for rows.Next() {
		var item EvidenceItem
		var spans json.RawMessage
		var primary, secondary, snippet, artifactData string
		if err := rows.Scan(&item.KnowledgeStoreID, &item.DocumentGroup, &item.DocumentID,
			&item.ArtifactID, &item.ArtifactType, &item.SourceTitle, &spans, &item.ValidationStatus,
			&item.SourceVersion, &item.SourceFingerprint,
			&primary, &secondary, &snippet, &artifactData); err != nil {
			return ToolOutput{}, err
		}
		item.Page, item.LineStart, item.LineEnd = firstLocation(spans)
		item.Content, _ = json.Marshal(map[string]string{"primary_label": primary, "secondary_label": secondary, "snippet": snippet, "artifact_data": artifactData})
		item.UntrustedEvidence = true
		out.Items = append(out.Items, item)
	}
	return out, rows.Err()
}

func (b *SQLKnowledgeToolBackend) documentContext(ctx context.Context, in ToolInput) (ToolOutput, error) {
	var item EvidenceItem
	var title, docNo, source, filename, publishDate, processingStatus string
	var artifactCount int
	err := b.DB.QueryRowContext(ctx, `SELECT ks.id::text, COALESCE(i.type, ''), i.id::text,
COALESCE(i.title, ''), COALESCE(i.doc_no, ''), COALESCE(i.source, ''), i.modify_time::text,
COALESCE(NULLIF(i.md5, ''), 'input:' || i.id::text || ':' || EXTRACT(EPOCH FROM i.modify_time)::bigint::text),
COALESCE(i.file_name, ''), COALESCE(i.publish_date::text, ''), LEFT(COALESCE(i.status::text, '[]'), 4000),
(SELECT COUNT(*) FROM kb.search_artifacts sa WHERE sa.input_record_id=i.id)
FROM kb.inputs i JOIN kb.knowledge_store ks ON ks.id=i.ks_store_id
WHERE ks.status='active' AND ks.id::text=$1 AND i.id::text=$2 AND ($3='' OR i.type=$3)`,
		in.KnowledgeStoreID, in.DocumentID, in.DocumentGroup).Scan(&item.KnowledgeStoreID, &item.DocumentGroup, &item.DocumentID, &title, &docNo, &source, &item.SourceVersion, &item.SourceFingerprint, &filename, &publishDate, &processingStatus, &artifactCount)
	if err != nil {
		return ToolOutput{}, err
	}
	item.SourceTitle = title
	item.ValidationStatus = "unknown"
	item.Content, _ = json.Marshal(map[string]any{"title": title, "document_number": docNo, "source": source, "filename": filename, "publish_date": publishDate, "processing_status": processingStatus, "artifact_count": artifactCount})
	item.UntrustedEvidence = true
	return ToolOutput{Items: []EvidenceItem{item}}, nil
}

func (b *SQLKnowledgeToolBackend) readPassages(ctx context.Context, in ToolInput) (ToolOutput, error) {
	var resultFile, title, group, store, document, version, fingerprint string
	err := b.DB.QueryRowContext(ctx, `SELECT i.result_filename, COALESCE(i.title,''), COALESCE(i.type,''), ks.id::text, i.id::text,
i.modify_time::text, COALESCE(NULLIF(i.md5, ''), 'input:' || i.id::text || ':' || EXTRACT(EPOCH FROM i.modify_time)::bigint::text)
FROM kb.inputs i JOIN kb.knowledge_store ks ON ks.id=i.ks_store_id
WHERE ks.status='active' AND ks.id::text=$1 AND i.id::text=$2 AND ($3='' OR i.type=$3)`,
		in.KnowledgeStoreID, in.DocumentID, in.DocumentGroup).Scan(&resultFile, &title, &group, &store, &document, &version, &fingerprint)
	if err != nil {
		return ToolOutput{}, err
	}
	resolved := pathutil.ResolveDataHomePath(resultFile)
	rawPath := filepath.Join(filepath.Dir(resolved), strings.TrimSuffix(filepath.Base(resolved), filepath.Ext(resolved))+".txt")
	f, err := os.Open(rawPath)
	if err != nil {
		return ToolOutput{}, err
	}
	defer f.Close()
	out := ToolOutput{Items: []EvidenceItem{}}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.SplitN(strings.TrimRight(scanner.Text(), "\r"), "\t", 7)
		if len(fields) != 7 {
			continue
		}
		line, lineErr := strconv.Atoi(strings.TrimSpace(fields[0]))
		if lineErr != nil || !lineRequested(line, in.Ranges) {
			continue
		}
		page, _ := strconv.Atoi(strings.TrimSpace(fields[1]))
		content, _ := json.Marshal(map[string]string{"line_type": strings.TrimSpace(fields[2]), "text": strings.TrimSpace(fields[6])})
		out.Items = append(out.Items, EvidenceItem{KnowledgeStoreID: store, DocumentGroup: group, DocumentID: document,
			SourceTitle: title, SourceVersion: version, SourceFingerprint: fingerprint, ValidationStatus: "unknown",
			Page: page, LineStart: line, LineEnd: line, UntrustedEvidence: true, Content: content})
		if len(out.Items) >= MaxToolLines {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return ToolOutput{}, err
	}
	return out, nil
}

func lineRequested(line int, ranges []LineRange) bool {
	for _, r := range ranges {
		if line >= r.Start && line <= r.End {
			return true
		}
	}
	return false
}

func firstLocation(raw json.RawMessage) (page, start, end int) {
	var stringSpans []string
	if json.Unmarshal(raw, &stringSpans) == nil && len(stringSpans) > 0 {
		parts := strings.SplitN(stringSpans[0], ":", 2)
		start, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
		end = start
		if len(parts) == 2 {
			end, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
		}
		return 0, start, end
	}
	var spans []map[string]any
	if json.Unmarshal(raw, &spans) != nil || len(spans) == 0 {
		return 0, 0, 0
	}
	number := func(keys ...string) int {
		for _, key := range keys {
			if v, ok := spans[0][key].(float64); ok {
				return int(v)
			}
		}
		return 0
	}
	page, start, end = number("page", "page_number"), number("start", "line_start", "start_line", "line_number"), number("end", "line_end", "end_line")
	if end == 0 {
		end = start
	}
	return
}

type KnowledgeToolService struct {
	backend KnowledgeToolBackend
	access  KnowledgeAccessChecker
}

func NewKnowledgeToolService(backend KnowledgeToolBackend, access KnowledgeAccessChecker) *KnowledgeToolService {
	return &KnowledgeToolService{backend: backend, access: access}
}

func (s *KnowledgeToolService) Execute(ctx context.Context, claims RunCapabilityClaims, tool string, input ToolInput) (ToolOutput, error) {
	if s == nil || s.backend == nil || s.access == nil || !stringIn(claims.AllowedTools, tool) {
		return ToolOutput{}, ErrKnowledgeAccessDenied
	}
	if err := validateToolInput(tool, &input); err != nil {
		return ToolOutput{}, err
	}
	if !scopeAllows(claims.KnowledgeStoreIDs, input.KnowledgeStoreID) ||
		(len(claims.DocumentIDs) > 0 && input.DocumentID == "") ||
		(input.DocumentID != "" && len(claims.DocumentIDs) > 0 && !scopeAllows(claims.DocumentIDs, input.DocumentID)) ||
		(len(claims.DocumentGroups) > 0 && input.DocumentGroup == "") ||
		(input.DocumentGroup != "" && len(claims.DocumentGroups) > 0 && !scopeAllows(claims.DocumentGroups, input.DocumentGroup)) {
		return ToolOutput{}, ErrKnowledgeAccessDenied
	}
	req := KnowledgeAccessRequest{UserID: claims.UserID, ProfileSlug: claims.ProfileSlug, ProfileVersion: claims.ProfileVersion,
		ToolName: tool, KnowledgeStoreID: input.KnowledgeStoreID, DocumentGroup: input.DocumentGroup, DocumentID: input.DocumentID}
	if err := s.access.CheckKnowledgeAccess(ctx, req); err != nil {
		return ToolOutput{}, ErrKnowledgeAccessDenied
	}
	out, err := s.backend.Execute(ctx, tool, input)
	if err != nil {
		return ToolOutput{}, err
	}
	for i := range out.Items {
		item := &out.Items[i]
		if item.KnowledgeStoreID == "" {
			item.KnowledgeStoreID = input.KnowledgeStoreID
		}
		if item.DocumentID == "" {
			item.DocumentID = input.DocumentID
		}
		if !scopeAllows(claims.KnowledgeStoreIDs, item.KnowledgeStoreID) || (len(claims.DocumentIDs) > 0 && !scopeAllows(claims.DocumentIDs, item.DocumentID)) {
			return ToolOutput{}, ErrKnowledgeAccessDenied
		}
		if len(claims.DocumentGroups) > 0 && !scopeAllows(claims.DocumentGroups, item.DocumentGroup) {
			return ToolOutput{}, ErrKnowledgeAccessDenied
		}
		if err := s.access.CheckKnowledgeAccess(ctx, KnowledgeAccessRequest{UserID: claims.UserID, ProfileSlug: claims.ProfileSlug,
			ProfileVersion: claims.ProfileVersion, ToolName: tool, KnowledgeStoreID: item.KnowledgeStoreID,
			DocumentGroup: item.DocumentGroup, DocumentID: item.DocumentID}); err != nil {
			return ToolOutput{}, ErrKnowledgeAccessDenied
		}
		item.UntrustedEvidence = true
		if item.ValidationStatus == "" {
			item.ValidationStatus = "unknown"
		}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return ToolOutput{}, fmt.Errorf("encode tool response: %w", err)
	}
	maxEvidenceBytes := claims.MaxEvidenceBytes
	if maxEvidenceBytes <= 0 || maxEvidenceBytes > MaxToolResponseBytes {
		maxEvidenceBytes = MaxToolResponseBytes
	}
	if len(encoded) > maxEvidenceBytes {
		return ToolOutput{}, ErrToolResponseTooLarge
	}
	return out, nil
}

func validateToolInput(tool string, input *ToolInput) error {
	if strings.TrimSpace(input.KnowledgeStoreID) == "" || !stringIn(knowledgeToolNames, tool) {
		return ErrInvalidToolInput
	}
	if len(input.Query) > MaxToolQueryBytes || len(input.Ranges) > MaxToolRanges {
		return ErrInvalidToolInput
	}
	if input.Limit <= 0 {
		input.Limit = 8
	}
	if input.Limit > MaxToolResults {
		return ErrInvalidToolInput
	}
	total := 0
	for _, r := range input.Ranges {
		if r.Start < 1 || r.End < r.Start {
			return ErrInvalidToolInput
		}
		total += r.End - r.Start + 1
	}
	if total > MaxToolLines {
		return ErrInvalidToolInput
	}
	switch tool {
	case "search_knowledge":
		if strings.TrimSpace(input.Query) == "" {
			return ErrInvalidToolInput
		}
	case "read_source_passages":
		if input.DocumentID == "" || len(input.Ranges) == 0 {
			return ErrInvalidToolInput
		}
	case "get_artifact_details":
		if input.ArtifactID == "" {
			return ErrInvalidToolInput
		}
	case "get_document_context":
		if input.DocumentID == "" {
			return ErrInvalidToolInput
		}
	case "find_related_knowledge":
		if input.DocumentID == "" && input.ArtifactID == "" {
			return ErrInvalidToolInput
		}
	}
	return nil
}

func scopeAllows(scope []string, value string) bool {
	return value != "" && stringIn(scope, value)
}
