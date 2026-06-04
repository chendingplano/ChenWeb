package kbhandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// metricWikiGenLocks serializes generation per (metric_id, lang) so concurrent
// first-hits do not generate the same page more than once.
var (
	metricWikiGenMu    sync.Mutex
	metricWikiGenLocks = map[string]*sync.Mutex{}
)

func lockMetricWikiGen(key string) func() {
	metricWikiGenMu.Lock()
	lk, ok := metricWikiGenLocks[key]
	if !ok {
		lk = &sync.Mutex{}
		metricWikiGenLocks[key] = lk
	}
	metricWikiGenMu.Unlock()
	lk.Lock()
	return lk.Unlock
}

type metricWikiResponse struct {
	Status    bool            `json:"status"`
	Generated bool            `json:"generated"`
	Page      json.RawMessage `json:"page"`
}

// GetMetricWiki handles GET /api/v1/kb/metrics/:metric_id/wiki?lang=<lang>.
//
// It returns the metric's cached wiki page JSON, generating it on first request
// when absent. metric_id has the form "<record_id>_<seqno>" and is required;
// an empty/invalid value is an error.
func GetMetricWiki(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_MWIKI_001")
	defer rc.Close()
	logger := rc.GetLogger()

	metricID := strings.TrimSpace(c.Param("metric_id"))
	recordID, _, err := parseMetricID(metricID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing or invalid metric_id (CWB_KB_MWIKI_010)",
		})
	}

	lang := strings.TrimSpace(c.QueryParam("lang"))
	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactDir == "" {
		logger.Error("missing ARTIFACT_DIR")
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "server is not configured for artifacts (CWB_KB_MWIKI_011)",
		})
	}

	pagePath, err := metricWikiPath(artifactDir, recordID, metricID, lang)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid wiki page request: %v (CWB_KB_MWIKI_012)", err),
		})
	}

	// Cache hit: return the saved page verbatim.
	if data, err := os.ReadFile(pagePath); err == nil {
		logger.Info("metric wiki cache hit", "metric_id", metricID, "lang", lang, "path", pagePath)
		return c.JSON(http.StatusOK, metricWikiResponse{
			Status:    true,
			Generated: false,
			Page:      json.RawMessage(data),
		})
	} else if !os.IsNotExist(err) {
		logger.Error("read metric wiki page failed", "metric_id", metricID, "path", pagePath, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read wiki page (CWB_KB_MWIKI_013)",
		})
	}

	// Cache miss: generate under a per-(metric,lang) lock so concurrent first
	// hits collapse into a single generation.
	if lang == "" {
		lang = "en"
	}
	unlock := lockMetricWikiGen(metricID + ":" + lang)
	defer unlock()

	// Double-check: another request may have generated the page while we waited.
	if data, err := os.ReadFile(pagePath); err == nil {
		return c.JSON(http.StatusOK, metricWikiResponse{Status: true, Generated: false, Page: json.RawMessage(data)})
	}

	logger.Info("metric wiki cache miss, generating", "metric_id", metricID, "lang", lang)
	page, err := buildMetricWikiPageFn(c.Request().Context(), ApiTypes.ProjectDBHandle, logger, recordID, metricID, lang)
	if err != nil {
		logger.Error("generate metric wiki page failed", "metric_id", metricID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("failed to generate wiki page (CWB_KB_MWIKI_030): %v", err),
		})
	}
	if err := saveMetricWikiPage(pagePath, page); err != nil {
		logger.Error("save metric wiki page failed", "metric_id", metricID, "path", pagePath, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to save wiki page (CWB_KB_MWIKI_031)",
		})
	}

	logger.Info("metric wiki page generated", "metric_id", metricID, "lang", lang, "path", pagePath)
	return c.JSON(http.StatusOK, metricWikiResponse{Status: true, Generated: true, Page: page})
}

// metricWikiLangs is the set of page languages supported for now. English is
// always generated first; other languages are produced by translation.
var metricWikiLangs = map[string]bool{"en": true, "zh-cn": true}

// metricWikiPath returns the cache path for a metric's wiki page JSON:
//
//	ARTIFACT_DIR/<floor(recordID/1000)>/<recordID>/wikipage_metric_<metricID>.<lang>.json
//
// The record directory is resolved with resolveRecordArtifactDir (shared with
// the other artifact handlers). An empty lang defaults to "en"; an unsupported
// lang is an error.
func metricWikiPath(artifactDir string, recordID int64, metricID, lang string) (string, error) {
	if lang == "" {
		lang = "en"
	}
	if !metricWikiLangs[lang] {
		return "", fmt.Errorf("unsupported wiki page language %q", lang)
	}
	recordDir, err := resolveRecordArtifactDir(artifactDir, recordID)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("wikipage_metric_%s.%s.json", metricID, lang)
	return filepath.Join(recordDir, name), nil
}

// parseMetricID splits a metric_id of the form "<record_id>_<seqno>" into its
// numeric parts. Both parts must be positive integers (record_id >= 1,
// seqno >= 1). The input is not trimmed: surrounding whitespace is an error.
//
// metric_id is assigned by the metrics extractor as "<record_id>_<seqno>" with
// seqno starting at 1, and is never null/empty for a persisted metric (see
// doc-processor/extract-metrics-spec.md). A malformed value is treated as an
// error by callers.
func parseMetricID(metricID string) (recordID int64, seqno int, err error) {
	idx := strings.LastIndex(metricID, "_")
	if idx <= 0 || idx == len(metricID)-1 {
		return 0, 0, fmt.Errorf("invalid metric_id %q: expected \"<record_id>_<seqno>\"", metricID)
	}
	ridPart := metricID[:idx]
	seqPart := metricID[idx+1:]

	rid, err := strconv.ParseInt(ridPart, 10, 64)
	if err != nil || rid <= 0 {
		return 0, 0, fmt.Errorf("invalid metric_id %q: record_id must be a positive integer", metricID)
	}
	seq, err := strconv.Atoi(seqPart)
	if err != nil || seq <= 0 {
		return 0, 0, fmt.Errorf("invalid metric_id %q: seqno must be a positive integer", metricID)
	}
	return rid, seq, nil
}
