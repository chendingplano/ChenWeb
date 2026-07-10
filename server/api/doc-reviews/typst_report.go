package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

var typstLogger = loggerutil.CreateDefaultLogger("DR14")

var passLabelMap = map[string]string{
	"P1": "Language & Style",
	"P2": "Structure & Organization",
	"P3": "Content Quality",
	"P4": "Consistency",
	"P5": "Technical & Compliance",
	"P6": "Meta & Process",
}

type reportLexicon struct {
	language string
	font     string
	labels   map[string]string
	passes   map[string]string
	aspects  map[string]string
}

func reportLexiconForLanguage(language string) reportLexicon {
	lex := reportLexicon{
		language: "en",
		font:     "Linux Libertine",
		labels: map[string]string{
			"report-title": "Document Review Report", "page": "Page", "finding": "Finding", "related-source-lines": "Related Source Lines",
			"source": "Source", "of": "of", "errors": "Errors", "explanation": "Explanation", "referenced-matching-metric-lines": "Referenced Matching Metric Lines",
			"matched-source": "Matched Source", "recommended-correction": "Recommended Correction", "comparison-analyses": "Comparison Analyses", "versus": "vs.",
			"no-findings-artifact": "No findings for this artifact.", "no-findings-aspect": "No findings for this aspect.", "overall-assessment": "Overall Assessment", "main-problem-analysis": "Main Problem Analysis",
			"guidelines-recommendations": "Guidelines and Recommendations", "no-references": "No references listed.", "table-of-contents": "Table of Contents",
			"basic-information": "Basic Information", "document-title": "Document Title", "document-id": "Document ID", "document-date": "Document Date",
			"reviewer": "Reviewer(s)", "review-date": "Review Date", "review-scope": "Review Scope", "review-results": "Review Results",
			"summary": "Summary", "statistics": "Statistics", "aspect": "Aspect", "findings": "Findings", "no-aspects": "No aspects defined.",
			"grounding-references": "Grounding References", "supporting-references": "Supporting References",
		},
		passes:  passLabelMap,
		aspects: map[string]string{},
	}
	for _, aspect := range ListAspects() {
		lex.aspects[aspect.Name] = aspect.Label
	}
	if language != "zh" && language != "zh-cn" && language != "zh-hans" {
		return lex
	}
	lex.language = "zh-cn"
	lex.font = "PingFang SC"
	lex.labels = map[string]string{
		"report-title": "文档审查报告", "page": "第", "finding": "发现", "related-source-lines": "相关源行",
		"source": "源", "of": "/", "errors": "错误", "explanation": "说明", "referenced-matching-metric-lines": "引用的匹配指标源行",
		"matched-source": "匹配源", "recommended-correction": "建议的修正", "comparison-analyses": "比较分析", "versus": "对比",
		"no-findings-artifact": "此对象没有发现问题。", "no-findings-aspect": "此审查项没有发现问题。", "overall-assessment": "总体评估", "main-problem-analysis": "主要问题分析",
		"guidelines-recommendations": "指南与建议", "no-references": "没有列出参考资料。", "table-of-contents": "目录",
		"basic-information": "基本信息", "document-title": "文档标题", "document-id": "文档 ID", "document-date": "文档日期",
		"reviewer": "审查人", "review-date": "审查日期", "review-scope": "审查范围", "review-results": "审查结果",
		"summary": "摘要", "statistics": "统计", "aspect": "审查项", "findings": "发现", "no-aspects": "未定义审查项。",
		"grounding-references": "依据参考资料", "supporting-references": "支持性参考资料",
	}
	lex.passes = map[string]string{"P1": "语言与风格", "P2": "结构与组织", "P3": "内容质量", "P4": "一致性", "P5": "技术与合规", "P6": "元数据与流程"}
	lex.aspects = map[string]string{
		"grammar_spelling": "语法与拼写", "tone_voice": "语气与文风", "formatting_consistency": "格式一致性", "readability": "可读性", "localization": "本地化",
		"logical_flow": "逻辑流程", "heading_hierarchy": "标题层级", "toc_accuracy": "目录准确性", "navigability": "可导航性", "section_balance": "章节平衡性", "modularity": "模块化",
		"completeness": "完整性", "correctness": "正确性", "clarity": "清晰度", "conciseness": "简洁性", "relevance": "相关性", "currency": "时效性", "examples": "示例", "diagrams": "图表", "testable_claims": "可验证声明", "evidence_rationale": "证据与依据",
		"internal_contradictions": "内部矛盾", "terminology_consistency": "术语一致性", "cross_reference_correctness": "交叉引用正确性", "requirement_traceability": "需求可追溯性",
		"technical_accuracy": "技术准确性", "assumptions": "假设", "prerequisites": "前提条件", "standards_compliance": "标准符合性", "legal_compliance": "法律合规性", "regulatory_compliance": "监管合规性", "internal_policy": "内部政策", "security": "安全性", "performance": "性能", "error_handling": "错误处理", "limitations": "局限性", "metrics": "指标一致性", "provisions": "条款一致性", "entities": "实体一致性", "inventory_items": "库存项目一致性",
		"version_history": "版本历史", "review_status": "审查状态", "ownership": "归属", "references": "参考资料", "related_documents": "相关文档", "confidentiality": "保密性", "sensitive_data": "敏感数据", "pii": "个人身份信息", "data_retention": "数据保留", "license_ip": "许可与知识产权",
	}
	return lex
}

// provisionsPass is the pass the provisions reviewer is configured under
// (doc-review.local.toml [reviewers.provisions]; ADR 2026063003). Used to
// force the provisions aspect-section to render when it has analyses but
// zero findings this run (ADR 2026070602 / ADR 2026062203 §1.2).
const provisionsPass = "P5"

// GenerateTypstReport generates a Typst source file from the review report and
// compiles it to PDF. Output is written under $DOC_REVIEW_REPORTS. If that env
// var is unset the function is a no-op. Errors are non-fatal: callers should
// log and continue.
func GenerateTypstReport(ctx context.Context, requestID int64, skeleton *ReportSkeleton, req *RequestStatus) error {
	outputDir := strings.TrimSpace(os.Getenv("DOC_REVIEW_REPORTS"))
	if outputDir == "" {
		typstLogger.Info("DOC_REVIEW_REPORTS not set; skipping PDF generation", "request_id", requestID)
		return nil
	}

	templatePath := strings.TrimSpace(os.Getenv("DOC_REVIEW_TEMPLATE_FILENAME"))
	if templatePath == "" {
		templatePath = "docs/doc-templates/template-document-report.typ"
	}
	absTemplatePath, err := filepath.Abs(templatePath)
	if err != nil {
		return fmt.Errorf("resolve template path %q: %w", templatePath, err)
	}
	if _, err := os.Stat(absTemplatePath); err != nil {
		return fmt.Errorf("template not found at %q: %w", absTemplatePath, err)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir %q: %w", outputDir, err)
	}

	stamp := time.Now().Format("20060102-1504")
	variants, err := buildTypstVariants(ctx, req, skeleton)
	if err != nil {
		return err
	}
	for _, variant := range variants {
		baseName := reviewReportArtifactBaseName(stamp, req.LatestRunID, variant.suffix)
		typPath := filepath.Join(outputDir, baseName+".typ")
		pdfPath := filepath.Join(outputDir, baseName+".pdf")

		src := buildTypstSource(variant.skeleton, req, variant.language, absTemplatePath, variant.provisionAnalyses)
		if err := os.WriteFile(typPath, []byte(src), 0o644); err != nil {
			return fmt.Errorf("write typst file %q: %w", typPath, err)
		}
		typstLogger.Info("typst source written", "request_id", requestID, "language", variant.language, "path", typPath)

		// Compile: --root / lets the #import use an absolute filesystem path.
		cmd := exec.CommandContext(ctx, "typst", "compile", "--root", "/", typPath, pdfPath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			typstLogger.Warn("typst compile failed",
				"request_id", requestID, "language", variant.language, "error", err, "output", string(out))
			return fmt.Errorf("typst compile (%s): %w\n%s", variant.language, err, string(out))
		}
		typstLogger.Info("PDF generated", "request_id", requestID, "language", variant.language, "path", pdfPath)
	}
	return nil
}

func reviewReportArtifactBaseName(stamp string, runID int64, suffix string) string {
	return fmt.Sprintf("%s-%d-%s", stamp, runID, suffix)
}

type typstVariant struct {
	language          string
	suffix            string
	skeleton          *ReportSkeleton
	provisionAnalyses map[string][]ProvisionAnalysis
}

func buildTypstVariants(ctx context.Context, req *RequestStatus, base *ReportSkeleton) ([]typstVariant, error) {
	findings, metadataByFindingID, err := loadReportFindingsWithMetadata(ctx, ApiTypes.ProjectDBHandle, req)
	if err != nil {
		return nil, fmt.Errorf("load localized findings: %w", err)
	}
	provisionAnalyses, err := loadProvisionAnalysesByRun(ctx, ApiTypes.ProjectDBHandle, req)
	if err != nil {
		return nil, fmt.Errorf("load provision analyses: %w", err)
	}
	languages := docReviewReportLanguagesFromEnv()
	var variants []typstVariant
	for _, language := range languages {
		variants = append(variants, typstVariant{
			language:          language,
			suffix:            reportLanguageSuffix(language),
			skeleton:          buildLocalizedTypstSkeleton(ctx, req, base, findings, metadataByFindingID, language),
			provisionAnalyses: provisionAnalyses,
		})
	}
	return variants, nil
}

func docReviewReportLanguagesFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv("DOC_REVIEW_REPORT_LANGUAGE"))
	if raw == "" {
		return []string{"en"}
	}

	var parsed []string
	if strings.HasPrefix(raw, "[") {
		var items []string
		if err := json.Unmarshal([]byte(raw), &items); err == nil {
			parsed = items
		}
	} else {
		var single string
		if err := json.Unmarshal([]byte(raw), &single); err == nil {
			parsed = []string{single}
		} else {
			parsed = []string{raw}
		}
	}

	seen := map[string]bool{}
	var out []string
	for _, language := range parsed {
		language = strings.ToLower(strings.TrimSpace(language))
		switch language {
		case "", "en", "en-us":
			language = "en"
		}
		if language == "" || seen[language] {
			continue
		}
		seen[language] = true
		out = append(out, language)
	}
	if len(out) == 0 {
		return []string{"en"}
	}
	return out
}

func reportLanguageSuffix(language string) string {
	switch language {
	case "zh", "zh-cn", "zh-hans":
		return "report-cn"
	default:
		return "report-" + language
	}
}

func loadReportFindingsWithMetadata(ctx context.Context, db *sql.DB, req *RequestStatus) ([]FindingItem, map[int64][]byte, error) {
	if db == nil || req == nil || req.InputRecordID == 0 || req.LatestRunID == 0 {
		return nil, nil, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, pass, aspect, severity, finding_type, title, description,
		       COALESCE(evidence,''), COALESCE(location,''), COALESCE(suggestion,''),
		       COALESCE(confidence,0), COALESCE(review_status,'pending'), COALESCE(metadata, '{}'::jsonb)::text,
		       COALESCE(artifact_id,'')
		FROM kb.doc_review_findings
		WHERE input_record_id = $1 AND run_id = $2
		ORDER BY CASE severity WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END, id ASC`,
		req.InputRecordID, req.LatestRunID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var findings []FindingItem
	metadataByFindingID := map[int64][]byte{}
	for rows.Next() {
		var finding FindingItem
		var metadata string
		if err := rows.Scan(&finding.ID, &finding.Pass, &finding.Aspect, &finding.Severity, &finding.FindingType,
			&finding.Title, &finding.Description, &finding.Evidence, &finding.Location, &finding.Suggestion,
			&finding.Confidence, &finding.ReviewStatus, &metadata, &finding.ArtifactID); err != nil {
			return nil, nil, err
		}
		applyFindingMetadata(&finding, []byte(metadata))
		findings = append(findings, finding)
		metadataByFindingID[finding.ID] = []byte(metadata)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return findings, metadataByFindingID, nil
}

func buildLocalizedTypstSkeleton(ctx context.Context, req *RequestStatus, base *ReportSkeleton, findings []FindingItem, metadataByFindingID map[int64][]byte, language string) *ReportSkeleton {
	if base == nil {
		return nil
	}
	if len(findings) == 0 {
		return base
	}

	localizedFindings := make([]FindingItem, 0, len(findings))
	for _, finding := range findings {
		if tr, ok := translationFromMetadata(metadataByFindingID[finding.ID], language); ok {
			finding = applyFindingTranslation(finding, tr)
		}
		localizedFindings = append(localizedFindings, finding)
	}

	gen := NewDocReviewReportGenerator()
	gen.DB = nil
	gen.PackageOrder = configuredPackageOrder()
	localized, err := gen.Build(ctx, req, localizedFindings)
	if err != nil {
		typstLogger.Warn("buildLocalizedTypstSkeleton: fallback to base skeleton", "language", language, "error", err)
		return base
	}
	localized.Meta = base.Meta
	copyLocalizedSources(localized, base)
	return localized
}

func copyLocalizedSources(dst, src *ReportSkeleton) {
	if dst == nil || src == nil {
		return
	}
	for i := range dst.Findings {
		if i < len(src.Findings) {
			dst.Findings[i].Sources = src.Findings[i].Sources
			dst.Findings[i].Related = src.Findings[i].Related
		}
	}
	for pass, dstGroup := range dst.FindingsByPass {
		srcGroup, ok := src.FindingsByPass[pass]
		if !ok {
			continue
		}
		for i := range dstGroup.Findings {
			if i < len(srcGroup.Findings) {
				dstGroup.Findings[i].Sources = srcGroup.Findings[i].Sources
				dstGroup.Findings[i].Related = srcGroup.Findings[i].Related
			}
		}
		dst.FindingsByPass[pass] = dstGroup
	}
}

// buildFindingBlock renders one review-finding(...) Typst call for f, using
// fid as its displayed id (either the DB finding id or an "F-NN" ordinal
// fallback — see findingDisplayID).
func buildFindingBlock(f ReportFinding, fid, labelsArg string) string {
	var blockB strings.Builder
	fmt.Fprintf(&blockB, "      review-finding(\n")
	fmt.Fprintf(&blockB, "        labels: %s,\n", labelsArg)
	fmt.Fprintf(&blockB, "        id: \"%s\",\n", typStr(fid))

	// Emit sources array — one dict per source location group.
	fmt.Fprintf(&blockB, "        sources: (")
	if len(f.Sources) > 0 {
		fmt.Fprintf(&blockB, "\n")
		for _, sc := range f.Sources {
			fmt.Fprintf(&blockB, "          (\n")
			if sc.Before != "" {
				fmt.Fprintf(&blockB, "            before: [%s],\n", typLines(sc.Before))
			} else {
				fmt.Fprintf(&blockB, "            before: none,\n")
			}
			fmt.Fprintf(&blockB, "            source: [%s],\n", typLines(sc.Source))
			if sc.After != "" {
				fmt.Fprintf(&blockB, "            after: [%s],\n", typLines(sc.After))
			} else {
				fmt.Fprintf(&blockB, "            after: none,\n")
			}
			fmt.Fprintf(&blockB, "          ),\n")
		}
		fmt.Fprintf(&blockB, "        ")
	}
	fmt.Fprintf(&blockB, "),\n")

	fmt.Fprintf(&blockB, "        related-sources: (")
	if len(f.Related) > 0 {
		fmt.Fprintf(&blockB, "\n")
		for _, sc := range f.Related {
			fmt.Fprintf(&blockB, "          (\n")
			if sc.Before != "" {
				fmt.Fprintf(&blockB, "            before: [%s],\n", typLines(sc.Before))
			} else {
				fmt.Fprintf(&blockB, "            before: none,\n")
			}
			fmt.Fprintf(&blockB, "            source: [%s],\n", typLines(sc.Source))
			if sc.After != "" {
				fmt.Fprintf(&blockB, "            after: [%s],\n", typLines(sc.After))
			} else {
				fmt.Fprintf(&blockB, "            after: none,\n")
			}
			fmt.Fprintf(&blockB, "          ),\n")
		}
		fmt.Fprintf(&blockB, "        ")
	}
	fmt.Fprintf(&blockB, "),\n")

	fmt.Fprintf(&blockB, "        errors: [%s],\n", typContent(f.Title))
	fmt.Fprintf(&blockB, "        explanation: [%s],\n", typContent(f.Description))
	fmt.Fprintf(&blockB, "        correction: [%s],\n", typContent(f.Suggestion))
	fmt.Fprintf(&blockB, "      ),")
	return blockB.String()
}

// findingDisplayID returns the DB finding id as a string, or an "F-NN"
// ordinal fallback when id is 0 (legacy report JSON with no finding row id).
func findingDisplayID(id int64, ordinal int) string {
	if id == 0 {
		return fmt.Sprintf("F-%02d", ordinal)
	}
	return fmt.Sprintf("%d", id)
}

func passOrderContains(passOrder []string, target string) bool {
	for _, p := range passOrder {
		if p == target {
			return true
		}
	}
	return false
}

// isArtifactAnchoredAspect reports whether aspect belongs to one of the
// per-artifact reviewers (ADR 2026070603), whose findings should be grouped
// by ArtifactID rather than rendered as a flat list.
func isArtifactAnchoredAspect(aspect string) bool {
	switch aspect {
	case "metrics", "provisions", "inventory_items":
		return true
	default:
		return false
	}
}

// buildArtifactGroupsArg renders the Typst artifact-group(...) array for one
// aspect-section: af is grouped by ArtifactID, ordered by each artifact's
// earliest kb.doc_review_findings.id (not by first appearance in the
// severity-sorted af slice). For the "provisions" aspect, artifact IDs that
// have analyses (provisionAnalyses)
// but no finding are unioned in (sorted, appended after the finding-derived
// IDs) so "no conflict" comparisons are still visible (ADR 2026070602 / ADR
// 2026062203 §1.2). findingIdx is the shared ordinal counter also used by the
// flat-findings path, threaded by pointer so numbering stays continuous.
func buildArtifactGroupsArg(af []ReportFinding, provisionAnalyses map[string][]ProvisionAnalysis, aspect string, findingIdx *int, labelsArg string) string {
	artifactFindingMap := map[string][]ReportFinding{}
	minFindingID := map[string]int64{}
	var artifactIDs []string
	seen := map[string]bool{}
	for _, f := range af {
		if !seen[f.ArtifactID] {
			seen[f.ArtifactID] = true
			artifactIDs = append(artifactIDs, f.ArtifactID)
			minFindingID[f.ArtifactID] = f.ID
		} else if f.ID < minFindingID[f.ArtifactID] {
			minFindingID[f.ArtifactID] = f.ID
		}
		artifactFindingMap[f.ArtifactID] = append(artifactFindingMap[f.ArtifactID], f)
	}
	// af arrives severity-first (see loadReportFindingsWithMetadata's ORDER BY),
	// which scrambles cross-artifact order. Re-order groups by each artifact's
	// earliest kb.doc_review_findings.id instead, so artifacts appear in the
	// order they were reviewed/created rather than by finding severity.
	sort.Slice(artifactIDs, func(i, j int) bool {
		return minFindingID[artifactIDs[i]] < minFindingID[artifactIDs[j]]
	})

	if aspect == "provisions" && len(provisionAnalyses) > 0 {
		analysisIDs := make([]string, 0, len(provisionAnalyses))
		for id := range provisionAnalyses {
			analysisIDs = append(analysisIDs, id)
		}
		sort.Strings(analysisIDs)
		for _, id := range analysisIDs {
			if !seen[id] {
				seen[id] = true
				artifactIDs = append(artifactIDs, id)
			}
		}
	}

	var groups []string
	for _, artifactID := range artifactIDs {
		var findingBlocks []string
		for _, f := range artifactFindingMap[artifactID] {
			*findingIdx++
			findingBlocks = append(findingBlocks, buildFindingBlock(f, findingDisplayID(f.ID, *findingIdx), labelsArg))
		}
		var findingsArg string
		if len(findingBlocks) > 0 {
			findingsArg = "\n" + strings.Join(findingBlocks, "\n") + "\n      "
		}

		var analysesArg string
		if entries := provisionAnalyses[artifactID]; aspect == "provisions" && len(entries) > 0 {
			var analysisBlocks []string
			for _, a := range entries {
				analysisBlocks = append(analysisBlocks, fmt.Sprintf(
					"        (related: \"%s\", relationship: \"%s\", summary: [%s]),",
					typStr(a.RelatedArtifactID), typStr(a.Relationship), typContent(a.Summary),
				))
			}
			analysesArg = "\n" + strings.Join(analysisBlocks, "\n") + "\n      "
		}

		title := artifactID
		if title == "" {
			title = "(unidentified artifact)"
		}
		groups = append(groups, fmt.Sprintf(
			"      artifact-group(\n"+
				"        labels: %s,\n"+
				"        title: \"%s\",\n"+
				"        analyses: (%s),\n"+
				"        findings: (%s),\n"+
				"      ),",
			labelsArg, typStr(title), analysesArg, findingsArg,
		))
	}
	if len(groups) == 0 {
		return ""
	}
	return "\n" + strings.Join(groups, "\n") + "\n    "
}

// buildTypstSource returns the full .typ source for the review report.
func buildTypstSource(skeleton *ReportSkeleton, req *RequestStatus, lang, absTemplatePath string, provisionAnalyses map[string][]ProvisionAnalysis) string {
	var b strings.Builder
	lex := reportLexiconForLanguage(lang)
	labelsArg := typstLabelsArg(lex.labels)
	typstLanguage := lex.language
	if typstLanguage == "zh-cn" {
		typstLanguage = "zh"
	}

	// Import template functions.  Module content output is discarded; only
	// the #let bindings become available in this document.
	fmt.Fprintf(&b, "#import \"%s\": *\n\n", absTemplatePath)

	// Propagate non-English language to the document level.  The template's
	// own set-text(lang:"en") is scoped inside the function body and will
	// override this for content the function generates; this line covers any
	// surrounding markup we might add in the future.
	if lex.language != "en" {
		fmt.Fprintf(&b, "#set text(lang: %q)\n\n", typstLanguage)
	}

	// ── Metadata ────────────────────────────────────────────────
	docTitle := skeleton.Meta.DocumentTitle
	docID := skeleton.Meta.ReportID
	reviewDate := extractDate(skeleton.Meta.GeneratedAt)
	reviewer := req.RequesterName
	if reviewer == "" {
		if lex.language == "zh-cn" {
			reviewer = "自动审查"
		} else {
			reviewer = "Automated Review"
		}
	}

	// Review scope: human-readable list of pass labels.
	var scopeParts []string
	for _, p := range skeleton.PassOrder {
		if label, ok := lex.passes[p]; ok {
			scopeParts = append(scopeParts, label)
		}
	}
	reviewScope := strings.Join(scopeParts, ", ")

	// ── aspect-stats ─────────────────────────────────────────────
	var statLines []string
	for _, p := range skeleton.PassOrder {
		pg := skeleton.FindingsByPass[p]
		label := lex.passes[p]
		statLines = append(statLines, fmt.Sprintf("    (aspect: \"%s\", count: %d),", typStr(label), len(pg.Findings)))
	}

	// ── Package-level sections (hierarchical: L1 package → L2 aspects) ──
	passOrder := skeleton.PassOrder
	if len(provisionAnalyses) > 0 && !passOrderContains(passOrder, provisionsPass) {
		passOrder = append(append([]string{}, passOrder...), provisionsPass)
	}

	var aspectLines []string
	findingIdx := 0
	for _, p := range passOrder {
		pg := skeleton.FindingsByPass[p]
		label, ok := lex.passes[p]
		if !ok {
			label = p
		}

		// ── Package heading (Level 1) ──
		aspectLines = append(aspectLines, fmt.Sprintf(`  heading(level: 1, "%s"),`, typStr(label)))

		// ── Group findings by aspect within this package ──
		aspectFindingMap := map[string][]ReportFinding{}
		var aspectSlugs []string
		aspectSeen := map[string]bool{}
		for _, f := range pg.Findings {
			if !aspectSeen[f.Aspect] {
				aspectSeen[f.Aspect] = true
				aspectSlugs = append(aspectSlugs, f.Aspect)
			}
			aspectFindingMap[f.Aspect] = append(aspectFindingMap[f.Aspect], f)
		}
		if p == provisionsPass && len(provisionAnalyses) > 0 && !aspectSeen["provisions"] {
			aspectSeen["provisions"] = true
			aspectSlugs = append(aspectSlugs, "provisions")
		}

		// ── One aspect-section per aspect (Level 2) ──
		for _, aspect := range aspectSlugs {
			af := aspectFindingMap[aspect]
			var findingBlocks []string
			var aspectHighCount int
			artifactAnchored := isArtifactAnchoredAspect(aspect)

			for _, f := range af {
				if f.Severity == "high" {
					aspectHighCount++
				}
				if artifactAnchored {
					continue
				}
				findingIdx++
				fid := findingDisplayID(f.ID, findingIdx)
				findingBlocks = append(findingBlocks, buildFindingBlock(f, fid, labelsArg))
			}

			assessment := buildAssessment(len(af), aspectHighCount, lex.language)
			problems := buildProblems([]string{aspect}, lex)
			guidelines := buildGuidelines(af, lex.language)

			var findingsArg, artifactGroupsArg string
			if artifactAnchored {
				artifactGroupsArg = buildArtifactGroupsArg(af, provisionAnalyses, aspect, &findingIdx, labelsArg)
			} else if len(findingBlocks) > 0 {
				findingsArg = "\n" + strings.Join(findingBlocks, "\n") + "\n    "
			}

			section := fmt.Sprintf(
				"    aspect-section(\n"+
					"      labels: %s,\n"+
					"      title: \"%s\",\n"+
					"      findings: (%s),\n"+
					"      artifact-groups: (%s),\n"+
					"      assessment: [%s],\n"+
					"      problems: [%s],\n"+
					"      guidelines: [%s],\n"+
					"    ),",
				labelsArg,
				typStr(firstNonEmpty(lex.aspects[aspect], aspect)),
				findingsArg,
				artifactGroupsArg,
				typContent(assessment),
				typContent(problems),
				typContent(guidelines),
			)
			aspectLines = append(aspectLines, section)
		}
	}

	// ── grounding refs (from reference documents) ─────────────────
	var groundingLines []string
	for i, rd := range req.ReferenceDocs {
		gid := fmt.Sprintf("[G%d]", i+1)
		groundingLines = append(groundingLines, fmt.Sprintf(
			"    (id: \"%s\", title: \"%s\", description: \"%s\"),",
			typStr(gid), typStr(rd.Title), typStr(rd.DocNo),
		))
	}

	// ── supporting refs (from high-severity recommendations) ───────
	var supportingLines []string
	for i, rec := range skeleton.Recommendations {
		sid := fmt.Sprintf("[S%d]", i+1)
		supportingLines = append(supportingLines, fmt.Sprintf(
			"    (id: \"%s\", title: \"%s\", description: \"\"),",
			typStr(sid), typStr(rec.Action),
		))
	}

	// ── Main template call ─────────────────────────────────────────
	fmt.Fprintf(&b, "#document-review-report(\n")
	fmt.Fprintf(&b, "  lang: %q,\n", typstLanguage)
	fmt.Fprintf(&b, "  font: %q,\n", lex.font)
	fmt.Fprintf(&b, "  labels: %s,\n", labelsArg)
	fmt.Fprintf(&b, "  doc-title: \"%s\",\n", typStr(docTitle))
	fmt.Fprintf(&b, "  doc-id: \"%s\",\n", typStr(docID))
	fmt.Fprintf(&b, "  doc-date: \"%s\",\n", typStr(reviewDate))
	fmt.Fprintf(&b, "  reviewer: \"%s\",\n", typStr(reviewer))
	fmt.Fprintf(&b, "  review-date: \"%s\",\n", typStr(reviewDate))
	fmt.Fprintf(&b, "  review-scope: \"%s\",\n", typStr(reviewScope))
	fmt.Fprintf(&b, "  summary: [%s],\n", typContent(localizedExecutiveSummary(skeleton, lex.language)))
	writeTypstArray(&b, "  aspect-stats", statLines)
	writeTypstArray(&b, "  aspects", aspectLines)
	writeTypstArray(&b, "  grounding-refs", groundingLines)
	writeTypstArray(&b, "  supporting-refs", supportingLines)
	fmt.Fprintf(&b, ")\n")

	return b.String()
}

func typstLabelsArg(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("(\n")
	for _, key := range keys {
		fmt.Fprintf(&b, "  %s: %q,\n", key, typStr(labels[key]))
	}
	b.WriteString(")")
	return b.String()
}

// writeTypstArray writes a named Typst array argument.
func writeTypstArray(b *strings.Builder, name string, lines []string) {
	if len(lines) == 0 {
		fmt.Fprintf(b, "%s: (),\n", name)
		return
	}
	fmt.Fprintf(b, "%s: (\n", name)
	for _, l := range lines {
		fmt.Fprintf(b, "%s\n", l)
	}
	fmt.Fprintf(b, "  ),\n")
}

/*
// buildRelated constructs the "related source lines" snippet from a finding.
func buildRelated(location, evidence string) string {
	switch {
	case location != "" && evidence != "":
		return location + ": " + evidence
	case location != "":
		return location
	case evidence != "":
		return evidence
	default:
		return "No specific location identified."
	}
}
*/

// buildAssessment produces the overall-assessment sentence for a pass section.
func buildAssessment(total, high int, language string) string {
	if language == "zh-cn" {
		if total == 0 {
			return "本节未发现问题。"
		}
		s := fmt.Sprintf("本节发现 %d 项问题。", total)
		if high > 0 {
			s += fmt.Sprintf("其中 %d 项为高严重性问题，需要立即关注。", high)
		}
		return s
	}
	if total == 0 {
		return "No issues found in this section."
	}
	s := fmt.Sprintf("Found %d finding(s) in this section.", total)
	if high > 0 {
		s += fmt.Sprintf(" %d high-severity issue(s) require immediate attention.", high)
	}
	return s
}

// localizedExecutiveSummary derives the Chinese PDF-only summary from the
// already-localized report structure. The persisted report JSON remains the
// canonical English representation used by existing API and Markdown callers.
func localizedExecutiveSummary(skeleton *ReportSkeleton, language string) string {
	if skeleton == nil || language != "zh-cn" {
		if skeleton == nil {
			return ""
		}
		return skeleton.ExecutiveSummary.Text
	}

	var high, medium, low int
	for _, finding := range skeleton.Findings {
		switch finding.Severity {
		case "high":
			high++
		case "medium":
			medium++
		default:
			low++
		}
	}
	total := len(skeleton.Findings)
	summary := fmt.Sprintf("已审查 %d 个审查项，覆盖 %d 个审查阶段。发现 %d 项问题（高严重性 %d 项，中等严重性 %d 项，低严重性 %d 项）。",
		total, len(skeleton.PassOrder), total, high, medium, low)
	if high > 0 {
		summary += fmt.Sprintf("%d 项高严重性问题需要立即关注。", high)
	}
	return summary
}

// buildProblems summarises the distinct aspects that have issues.
func buildProblems(aspects []string, lex reportLexicon) string {
	localized := make([]string, 0, len(aspects))
	for _, aspect := range aspects {
		localized = append(localized, firstNonEmpty(lex.aspects[aspect], aspect))
	}
	if lex.language == "zh-cn" {
		if len(localized) == 0 {
			return "未发现主要问题。"
		}
		return "发现的问题：" + strings.Join(localized, "、") + "。"
	}
	if len(aspects) == 0 {
		return "No major issues identified."
	}
	return "Issues found in: " + strings.Join(localized, ", ") + "."
}

// buildGuidelines returns bullet-point guidelines from high-severity suggestions.
func buildGuidelines(findings []ReportFinding, language string) string {
	var lines []string
	for _, f := range findings {
		if f.Severity == "high" && strings.TrimSpace(f.Suggestion) != "" {
			lines = append(lines, "- "+f.Suggestion)
		}
	}
	if len(lines) == 0 {
		if language == "zh-cn" {
			return "请持续关注此审查项。"
		}
		return "Continue monitoring this area."
	}
	return strings.Join(lines, "\n")
}

// typStr escapes s for use inside a Typst string literal (between "").
func typStr(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// typContentLine escapes one line of text for use inside a Typst content block.
// Backslash is escaped first to prevent double-escaping. Every Typst markup
// metacharacter must be escaped, including the paired emphasis/raw delimiters
// (`*`, `_`, “ ` “): an odd, unbalanced count of these in finding text (e.g.
// `0.*` / `1.*` wildcard ranges) otherwise opens a delimiter Typst never closes,
// failing compilation with "unclosed delimiter".
func typContentLine(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `$`, `\$`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `[`, `\[`)
	s = strings.ReplaceAll(s, `]`, `\]`)
	s = strings.ReplaceAll(s, `#`, `\#`)
	s = strings.ReplaceAll(s, `@`, `\@`)
	s = strings.ReplaceAll(s, `<`, `\<`)
	s = strings.ReplaceAll(s, `>`, `\>`)
	s = strings.ReplaceAll(s, `*`, `\*`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, `~`, `\~`)
	return s
}

// typContent escapes a single-line string for use inside a Typst content block ([]).
// HTML <table> elements are converted to Typst #table() calls instead of being escaped.
func typContent(s string) string { return typContentWithHTML(s) }

// typLines escapes a multi-line string for use inside a Typst content block.
// Each line is escaped individually; lines are joined with a Typst forced line
// break (backslash + newline) so each line renders on its own row.
// HTML <table> elements are converted to Typst #table() calls instead of being escaped.
func typLines(s string) string {
	re := htmlTableRe()
	var result strings.Builder
	lastEnd := 0
	for _, loc := range re.FindAllStringIndex(s, -1) {
		segment := s[lastEnd:loc[0]]
		if segment != "" {
			result.WriteString(escapedLines(segment))
			result.WriteString("\\\n")
		}
		result.WriteString(htmlTableToTypst(s[loc[0]:loc[1]]))
		lastEnd = loc[1]
	}
	if remaining := s[lastEnd:]; remaining != "" {
		result.WriteString(escapedLines(remaining))
	}
	return result.String()
}

// escapedLines is the original typLines logic: escape each non-empty line and
// join with Typst forced line breaks.
func escapedLines(s string) string {
	lines := strings.Split(s, "\n")
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		if esc := typContentLine(line); esc != "" {
			parts = append(parts, esc)
		}
	}
	return strings.Join(parts, "\\\n")
}

// typContentWithHTML escapes s for a Typst content block, converting any
// embedded HTML <table> elements to Typst #table() calls.
func typContentWithHTML(s string) string {
	re := htmlTableRe()
	var result strings.Builder
	lastEnd := 0
	for _, loc := range re.FindAllStringIndex(s, -1) {
		result.WriteString(typContentLine(s[lastEnd:loc[0]]))
		result.WriteString(htmlTableToTypst(s[loc[0]:loc[1]]))
		lastEnd = loc[1]
	}
	result.WriteString(typContentLine(s[lastEnd:]))
	return result.String()
}

var (
	reHTMLTable = regexp.MustCompile(`(?is)<table[^>]*>.*?</table>`)
	reTableRow  = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	reTableCell = regexp.MustCompile(`(?is)<t[dh][^>]*>(.*?)</t[dh]>`)
	reAnyTag    = regexp.MustCompile(`<[^>]+>`)
)

func htmlTableRe() *regexp.Regexp { return reHTMLTable }

// htmlTableToTypst converts one HTML <table>...</table> string to a Typst
// #table() call. Cell content has inner tags stripped and HTML entities decoded
// before being escaped for Typst.
func htmlTableToTypst(tableHTML string) string {
	var allRows [][]string
	maxCols := 0
	for _, rowMatch := range reTableRow.FindAllStringSubmatch(tableHTML, -1) {
		var row []string
		for _, cellMatch := range reTableCell.FindAllStringSubmatch(rowMatch[1], -1) {
			cell := reAnyTag.ReplaceAllString(cellMatch[1], "")
			cell = decodeHTMLEntities(strings.TrimSpace(cell))
			row = append(row, cell)
		}
		if len(row) > 0 {
			allRows = append(allRows, row)
			if len(row) > maxCols {
				maxCols = len(row)
			}
		}
	}
	if maxCols == 0 {
		return typContentLine(reAnyTag.ReplaceAllString(tableHTML, ""))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#table(\n  columns: %d,\n", maxCols)
	for _, row := range allRows {
		for _, cell := range row {
			fmt.Fprintf(&b, "  [%s],", typContentLine(cell))
		}
		b.WriteByte('\n')
	}
	b.WriteString(")")
	return b.String()
}

// decodeHTMLEntities replaces common HTML entities with their text equivalents.
func decodeHTMLEntities(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	return s
}

// extractDate returns the YYYY-MM-DD prefix of an RFC3339 or similar timestamp.
func extractDate(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}
