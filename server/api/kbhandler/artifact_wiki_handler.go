package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

var getArtifactWikiMetricPayloadFn = buildArtifactWikiMetricPayload
var buildArtifactWikiMetricArticleFn = generateArtifactWikiMetricArticle

func GetArtifactWiki(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_AWIKI_001")
	defer rc.Close()
	logger := rc.GetLogger()

	artifactType := strings.TrimSpace(c.QueryParam("artifact_type"))
	if artifactType == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "artifact_type is required (CWB_KB_AWIKI_010)",
		})
	}
	artifactID := strings.TrimSpace(c.QueryParam("artifact_id"))
	if artifactID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "artifact_id is required (CWB_KB_AWIKI_011)",
		})
	}
	lang := normalizeArtifactWikiLang(c.QueryParam("lang"))
	if !metricWikiLangs[lang] {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("unsupported wiki page language %q (CWB_KB_AWIKI_012)", lang),
		})
	}
	includeArticle := strings.TrimSpace(strings.ToLower(c.QueryParam("include_article")))
	shouldIncludeArticle := includeArticle != "0" && includeArticle != "false"

	var (
		payload     artifactWikiPayload
		artifactDir string
		provider    artifactWikiProvider
		err         error
	)
	provider, ok := getArtifactWikiProvider(artifactType)
	if !ok {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("unsupported artifact_type %q (CWB_KB_AWIKI_013)", artifactType),
		})
	}
	payload, err = provider.loadPayload(c.Request().Context(), ApiTypes.ProjectDBHandle, logger, artifactID, lang)
	if err != nil {
		logger.Error("load artifact wiki payload failed", "artifact_type", artifactType, "artifact_id", artifactID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to load artifact wiki payload (CWB_KB_AWIKI_020)",
		})
	}
	artifactDir = strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if _, err = artifactWikiPagePath(artifactDir, artifactType, payload.RecordID, payload.ArtifactID, lang); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid artifact wiki path: %v (CWB_KB_AWIKI_021)", err),
		})
	}

	var article json.RawMessage
	fresh := false
	if shouldIncludeArticle {
		article, fresh, err = loadOrCreateArtifactWikiArticle(c.Request().Context(), ApiTypes.ProjectDBHandle, logger, artifactDir, artifactType, payload, lang)
		if err != nil {
			logger.Error("load artifact wiki article failed", "artifact_type", artifactType, "artifact_id", payload.ArtifactID, "lang", lang, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status:   false,
				ErrorMsg: "failed to load artifact wiki article (CWB_KB_AWIKI_022)",
			})
		}
	}

	return c.JSON(http.StatusOK, artifactWikiResponse{
		Status:         true,
		Fresh:          fresh,
		ArtifactType:   artifactType,
		ArtifactID:     payload.ArtifactID,
		Article:        json.RawMessage(article),
		Record:         payload.Record,
		SourceDocument: payload.SourceDocument,
		Generated:      payload.Generated,
	})
}

func normalizeArtifactWikiLang(lang string) string {
	lang = strings.TrimSpace(strings.ToLower(lang))
	if lang == "" {
		return "en"
	}
	return lang
}

func artifactWikiPagePath(artifactDir, artifactType string, recordID int64, artifactID, lang string) (string, error) {
	recordDir, err := resolveRecordArtifactDir(artifactDir, recordID)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("wikipage_%s_%s.%s.json", strings.TrimSpace(artifactType), strings.TrimSpace(artifactID), normalizeArtifactWikiLang(lang))
	return filepath.Join(recordDir, name), nil
}

func buildArtifactWikiMetricPayload(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
	mctx, err := compileMetricWikiContext(db, 0, artifactID)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	recordJSON, err := json.Marshal(mctx.Metric)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	docJSON, err := json.Marshal(mctx.Document)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	return artifactWikiPayload{
		RecordID:       mctx.RecordID,
		ArtifactID:     mctx.MetricID,
		Record:         json.RawMessage(recordJSON),
		SourceDocument: json.RawMessage(docJSON),
		Generated: artifactWikiGeneratedMeta{
			Lang:       normalizeArtifactWikiLang(lang),
			SourceHash: metricWikiSourceHash(mctx),
		},
	}, nil
}

func generateArtifactWikiMetricArticle(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (json.RawMessage, error) {
	recordID, _, err := parseMetricID(strings.TrimSpace(artifactID))
	if err != nil {
		return nil, err
	}
	return buildMetricWikiPageFn(ctx, db, logger, recordID, artifactID, lang)
}

func shouldGenerateMetricArtifactWikiArticleDirectly(lang string) bool {
	lang = normalizeArtifactWikiLang(lang)
	return lang != "en" && strings.TrimSpace(os.Getenv("TRANSLATION_MODEL_NAME")) == ""
}

func shouldGenerateGenericArtifactWikiArticleDirectly(lang string) bool {
	lang = normalizeArtifactWikiLang(lang)
	return lang != "en" && strings.TrimSpace(os.Getenv("TRANSLATION_MODEL_NAME")) == ""
}

func shouldRefreshGenericArtifactWikiCachedPageForLanguage(data []byte, lang string) bool {
	lang = strings.TrimSpace(strings.ToLower(lang))
	if lang != "zh-cn" {
		return false
	}

	var page genericArtifactWikiPage
	if err := json.Unmarshal(data, &page); err != nil {
		return false
	}

	prose := strings.Join([]string{
		page.Title,
		page.Lead,
		page.Definition,
		page.Background,
		page.HowUsed,
		page.ChoosingValues,
	}, " ")
	prose = strings.TrimSpace(prose)
	if prose == "" {
		return false
	}
	if containsCJKText(prose) {
		return false
	}
	return strings.ContainsAny(prose, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
}

func loadOrCreateMetricArtifactWikiArticle(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactDir string, recordID int64, artifactID, lang string) (json.RawMessage, bool, error) {
	targetPath, err := artifactWikiPagePath(artifactDir, "metric", recordID, artifactID, lang)
	if err != nil {
		return nil, false, err
	}
	if article, readErr := os.ReadFile(targetPath); readErr == nil {
		return json.RawMessage(article), false, nil
	} else if !os.IsNotExist(readErr) {
		return nil, false, readErr
	}

	if shouldGenerateMetricArtifactWikiArticleDirectly(lang) {
		article, genErr := buildArtifactWikiMetricArticleFn(ctx, db, logger, artifactID, lang)
		if genErr != nil {
			return nil, false, genErr
		}
		if saveErr := saveMetricWikiPage(targetPath, article); saveErr != nil {
			return nil, false, saveErr
		}
		return article, true, nil
	}

	if lang == "en" {
		article, genErr := buildArtifactWikiMetricArticleFn(ctx, db, logger, artifactID, lang)
		if genErr != nil {
			return nil, false, genErr
		}
		if saveErr := saveMetricWikiPage(targetPath, article); saveErr != nil {
			return nil, false, saveErr
		}
		return article, true, nil
	}

	englishPath, err := artifactWikiPagePath(artifactDir, "metric", recordID, artifactID, "en")
	if err != nil {
		return nil, false, err
	}
	englishArticle, readErr := os.ReadFile(englishPath)
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			return nil, false, readErr
		}
		englishArticle, err = buildArtifactWikiMetricArticleFn(ctx, db, logger, artifactID, "en")
		if err != nil {
			return nil, false, err
		}
		if err := saveMetricWikiPage(englishPath, englishArticle); err != nil {
			return nil, false, err
		}
	}

	translatedArticle, err := translateArtifactWikiMetricArticleFn(ctx, logger, json.RawMessage(englishArticle), lang)
	if err != nil {
		return nil, false, err
	}
	if err := saveMetricWikiPage(targetPath, translatedArticle); err != nil {
		return nil, false, err
	}
	return translatedArticle, true, nil
}

func loadOrCreateArtifactWikiArticle(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactDir, artifactType string, payload artifactWikiPayload, lang string) (json.RawMessage, bool, error) {
	if artifactType == "metric" {
		return loadOrCreateMetricArtifactWikiArticle(ctx, db, logger, artifactDir, payload.RecordID, payload.ArtifactID, lang)
	}

	targetPath, err := artifactWikiPagePath(artifactDir, artifactType, payload.RecordID, payload.ArtifactID, lang)
	if err != nil {
		return nil, false, err
	}
	needsRefresh := false
	if article, readErr := os.ReadFile(targetPath); readErr == nil {
		if shouldRefreshGenericArtifactWikiCachedPageForLanguage(article, lang) {
			needsRefresh = true
		} else {
			return json.RawMessage(article), false, nil
		}
	} else if !os.IsNotExist(readErr) {
		return nil, false, readErr
	}

	if shouldGenerateGenericArtifactWikiArticleDirectly(lang) {
		article, genErr := buildGenericArtifactWikiArticleFn(ctx, logger, payload, artifactType, lang)
		if genErr != nil {
			return nil, false, genErr
		}
		if saveErr := saveMetricWikiPage(targetPath, article); saveErr != nil {
			return nil, false, saveErr
		}
		return article, true, nil
	}

	englishPath, err := artifactWikiPagePath(artifactDir, artifactType, payload.RecordID, payload.ArtifactID, "en")
	if err != nil {
		return nil, false, err
	}
	var englishArticle json.RawMessage
	if article, readErr := os.ReadFile(englishPath); readErr == nil {
		englishArticle = json.RawMessage(article)
	} else if !os.IsNotExist(readErr) {
		return nil, false, readErr
	} else {
		englishArticle, err = buildGenericArtifactWikiArticleFn(ctx, logger, payload, artifactType, "en")
		if err != nil {
			return nil, false, err
		}
		if err := saveMetricWikiPage(englishPath, englishArticle); err != nil {
			return nil, false, err
		}
	}

	if lang == "en" {
		if targetPath != englishPath {
			if err := saveMetricWikiPage(targetPath, englishArticle); err != nil {
				return nil, false, err
			}
		}
		return englishArticle, !needsRefresh || targetPath == englishPath || needsRefresh, nil
	}

	translatedArticle, err := translateGenericArtifactWikiArticleFn(ctx, logger, englishArticle, lang)
	if err != nil {
		return nil, false, err
	}
	if err := saveMetricWikiPage(targetPath, translatedArticle); err != nil {
		return nil, false, err
	}
	return translatedArticle, true, nil
}
