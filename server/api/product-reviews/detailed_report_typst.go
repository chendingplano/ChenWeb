package productreviews

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

var productReportChinese = map[string]string{
	"metric name": "指标名称", "metric name en": "指标英文名称", "metric subject": "指标主体",
	"Node": "节点", "Kind": "类型", "Grounding": "关联依据", "Metrics": "指标数", "Documents": "文档数",
	"metric subject en": "指标英文主体", "metric value": "指标值", "metric unit": "指标单位",
	"threshold or target": "阈值或目标", "measurement frequency": "测量频率", "condition": "条件",
	"condition en": "英文条件", "metric desc": "指标说明", "metric context": "指标上下文",
	"value class": "值类别", "value data type": "值数据类型", "value range type": "值范围类型",
	"artifact id": "对象标识", "artifact type": "对象类型", "input record id": "文档记录编号",
	"source row id": "源记录编号", "source line spans": "原文引用行", "source table rows": "原文表格引用",
	"inclusion reason": "纳入原因", "tier": "归属层级", "score": "匹配得分", "paths": "检索路径",
	"node id": "节点编号", "label": "名称", "kind": "类型", "grounding": "关联依据",
	"artifact count": "对象数量", "document count": "文档数量", "title": "标题",
	"document number": "文档编号", "file name": "文件名", "publish date": "发布日期",
	"document type": "文档类型", "source language": "原文语言", "document metadata": "文档元数据",
	"Product Review Report": "产品审查报告", "Table of Contents": "目录", "Basic Information": "基本信息",
	"Product": "产品", "Run": "运行", "Created": "创建时间", "Reviewer": "审查人", "Review Date": "审查日期",
	"Review Scope": "审查范围", "Review Results": "审查结果", "Summary": "摘要", "Statistics": "统计",
	"Category": "类别", "Count": "数量", "Page": "页", "Source Documents": "源文档",
	"Supporting References": "支持性参考资料", "No references listed.": "未列出参考资料。",
	"Saved run": "已保存的运行", "Coverage by scope node": "范围节点覆盖情况", "Coverage gaps": "覆盖缺口",
	"Truncation details": "截断详情", "All result details": "全部结果详情", "Document scope": "文档范围",
	"Other scope nodes": "其他范围节点", "Metric fields (current record)": "指标字段（当前记录）",
	"Saved match details": "已保存的匹配详情", "Source evidence": "原文依据", "Field": "字段", "Value": "值",
	"Source document": "源文档", "Document metadata (current record)": "文档元数据（当前记录）",
	"Source document details":    "源文档详情",
	"Changes since previous run": "与上次运行的差异",
	"Saved document match":       "已保存的文档匹配", "Profile and request details": "配置与请求详情",
	"Request": "请求", "Product profile (current)": "产品配置（当前）", "Scope nodes (current)": "范围节点（当前）",
	"Attributed results": "已归属结果", "Document-scope results": "文档级结果", "Results": "结果",
	"Documents in scope": "范围内文档", "Truncated results": "截断结果", "None": "无",
	"Source text is unavailable or some cited lines could not be resolved. The saved citations are retained above.":                                                                                                             "原文不可用，或部分引用行无法解析。上方保留了已保存的引用。",
	"The metric record is unavailable; all saved result details are retained above.":                                                                                                                                            "指标记录不可用；上方保留了全部已保存的结果详情。",
	"All saved results are included, regardless of screen filters. Truncated results were excluded during the original run and cannot be reconstructed by this export.":                                                         "本报告包含全部已保存结果，不受页面筛选影响。截断结果在原始运行中已被排除，无法通过导出还原。",
	"Counts, coverage, scores, citations and match reasons are saved run data. Metric fields, source text, document metadata and the profile appendix are current records, not historical snapshots. Empty fields are omitted.": "计数、覆盖情况、得分、引用和匹配原因来自已保存的运行。指标字段、原文、文档元数据和附录中的产品配置取自当前记录，并非历史快照。空字段已省略。",
}

func reportLabel(label, lang string) string {
	if lang == "zh-cn" {
		if v := productReportChinese[label]; v != "" {
			return v
		}
	}
	return label
}

// Every external value is a quoted Typst string, never executable markup.
func reportQuote(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return ' '
		}
		return r
	}, s)
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`).Replace(s) + `"`
}

func reportObject(v any) map[string]any {
	raw, _ := json.Marshal(v)
	var fields map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	_ = dec.Decode(&fields)
	return fields
}

func reportValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	raw, _ := json.MarshalIndent(v, "", "  ")
	if string(raw) == "[]" || string(raw) == "{}" || string(raw) == "null" {
		return ""
	}
	return string(raw)
}

func reportFields(b *strings.Builder, value any, lang string) {
	fields := reportObject(value)
	keys := make([]string, 0, len(fields))
	for key, value := range fields {
		if reportValue(value) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	// Put the metric's meaning and requirement before bookkeeping fields.
	priority := map[string]int{}
	for i, key := range []string{"metric_name", "metric_name_en", "metric_subject", "metric_subject_en",
		"metric_value", "metric_unit", "metric_unit_en", "threshold_or_target", "condition", "condition_en",
		"metric_desc", "metric_desc_en", "metric_context", "metric_context_en"} {
		priority[key] = i + 1
	}
	sort.SliceStable(keys, func(i, j int) bool {
		a, b := priority[keys[i]], priority[keys[j]]
		if a == 0 {
			a = len(priority) + 1
		}
		if b == 0 {
			b = len(priority) + 1
		}
		return a < b
	})
	if len(keys) == 0 {
		return
	}
	fmt.Fprintf(b, "#table(columns: (32%%, 68%%), inset: 5pt, stroke: 0.4pt + clr-divider,\n table.header(text(weight: \"bold\", %s), text(weight: \"bold\", %s)),\n",
		reportQuote(reportLabel("Field", lang)), reportQuote(reportLabel("Value", lang)))
	for _, key := range keys {
		fmt.Fprintf(b, "text(size: 8pt, %s), text(size: 8pt, %s),\n", reportQuote(reportLabel(strings.ReplaceAll(key, "_", " "), lang)), reportQuote(reportValue(fields[key])))
	}
	b.WriteString(")\n\n")
}

func reportHeading(b *strings.Builder, level int, title, lang string) {
	fmt.Fprintf(b, "#heading(level: %d, %s)\n", level, reportQuote(reportLabel(title, lang)))
}

// renderDetailedReportTypst uses the same cover, contents, running headers,
// numbered sections and source blocks as Document Review.
func renderDetailedReportTypst(d *DetailedReport, templatePath, lang string) string {
	if lang != "zh-cn" {
		lang = "en"
	}
	var body strings.Builder
	reportHeading(&body, 1, "Saved run", lang)
	// report_md is the old summary, not the detailed report or a separate source of truth.
	runFields := reportObject(d.Run)
	delete(runFields, "report_md")
	delete(runFields, "report_json")
	reportFields(&body, runFields, lang)
	reportHeading(&body, 2, "Coverage by scope node", lang)
	body.WriteString("#table(columns: (2fr, 1fr, 1fr, auto, auto), inset: 5pt, stroke: 0.4pt + clr-divider, table.header(\n")
	for _, label := range []string{"Node", "Kind", "Grounding", "Metrics", "Documents"} {
		fmt.Fprintf(&body, "text(size: 8pt, weight: \"bold\", %s),\n", reportQuote(reportLabel(label, lang)))
	}
	body.WriteString("),\n")
	for _, node := range d.Summary.Coverage {
		for _, value := range []string{fmt.Sprintf("%s (#%d)", node.Label, node.NodeID), node.Kind, node.Grounding,
			fmt.Sprint(node.ArtifactCount), fmt.Sprint(node.DocumentCount)} {
			fmt.Fprintf(&body, "text(size: 8pt, %s),\n", reportQuote(value))
		}
	}
	body.WriteString(")\n")
	reportHeading(&body, 2, "Coverage gaps", lang)
	if len(d.Summary.Gaps) == 0 {
		fmt.Fprintf(&body, "#text(%s)\n", reportQuote(reportLabel("None", lang)))
	}
	for _, gap := range d.Summary.Gaps {
		reportFields(&body, gap, lang)
	}
	reportHeading(&body, 2, "Truncation details", lang)
	reportFields(&body, map[string]any{"truncated_count": d.Summary.TruncatedCount,
		"per_document_truncated": d.Summary.PerDocumentTruncated}, lang)
	if d.Diff != nil {
		reportHeading(&body, 2, "Changes since previous run", lang)
		reportFields(&body, d.Diff, lang)
	}

	reportHeading(&body, 1, "All result details", lang)
	docs := map[int64]reportDocument{}
	for _, doc := range d.Documents {
		docs[doc.ID] = doc
	}
	groups := map[int64][]Result{}
	for _, r := range d.Results {
		id := int64(0)
		if r.NodeID != nil {
			id = *r.NodeID
		}
		groups[id] = append(groups[id], r)
	}
	// Saved coverage labels are authoritative for this run, including deleted nodes.
	var order []int64
	labels := map[int64]string{0: reportLabel("Document scope", lang)}
	for _, node := range d.Summary.Coverage {
		order = append(order, node.NodeID)
		labels[node.NodeID] = node.Label
	}
	var extra []int64
	for id := range groups {
		if _, ok := labels[id]; !ok {
			extra = append(extra, id)
		}
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i] < extra[j] })
	order = append(append(order, extra...), 0)
	for _, id := range order {
		if len(groups[id]) == 0 {
			continue
		}
		label := labels[id]
		if label == "" {
			label = fmt.Sprintf("%s #%d", reportLabel("Other scope nodes", lang), id)
		}
		reportHeading(&body, 2, label, lang)
		for _, r := range groups[id] {
			name := r.MetricName
			if lang == "en" && r.MetricNameEn != "" {
				name = r.MetricNameEn
			}
			reportHeading(&body, 3, r.ArtifactID+" · "+name, lang)
			reportHeading(&body, 4, "Saved match details", lang)
			// Display fields on Result are joined from current metrics, so render
			// them only in the current-record block below.
			reportFields(&body, map[string]any{"artifact_type": r.ArtifactType, "artifact_id": r.ArtifactID,
				"source_row_id": r.SourceRowID, "input_record_id": r.InputRecordID, "node_id": r.NodeID,
				"tier": r.Tier, "score": r.Score, "paths": r.Paths, "inclusion_reason": r.InclusionReason,
				"source_line_spans": r.LineSpans}, lang)
			reportHeading(&body, 4, "Metric fields (current record)", lang)
			metric := d.Metrics[r.ArtifactID]
			if len(metric) == 0 || string(metric) == "null" {
				fmt.Fprintf(&body, "#text(%s)\n", reportQuote(reportLabel("The metric record is unavailable; all saved result details are retained above.", lang)))
			} else {
				reportFields(&body, metric, lang)
			}
			reportHeading(&body, 4, "Source evidence", lang)
			doc := docs[r.InputRecordID]
			fmt.Fprintf(&body, "#text(%s)\n\n", reportQuote(fmt.Sprintf("%s #%d · %s", reportLabel("Source document", lang), doc.ID, reportValue(doc.Metadata["title"]))))
			text, complete := reportSourceText(doc.Lines, r.LineSpans)
			if text != "" {
				fmt.Fprintf(&body, "#source-with-context(source: text(%s))\n", reportQuote(text))
			}
			if !complete || doc.Warning != "" {
				fmt.Fprintf(&body, "#text(fill: clr-error-fg, %s)\n", reportQuote(reportLabel("Source text is unavailable or some cited lines could not be resolved. The saved citations are retained above.", lang)))
			}
		}
	}
	reportHeading(&body, 1, "Source document details", lang)
	for _, doc := range d.Documents {
		reportHeading(&body, 2, fmt.Sprintf("#%d · %s", doc.ID, reportValue(doc.Metadata["title"])), lang)
		reportHeading(&body, 3, "Document metadata (current record)", lang)
		reportFields(&body, doc.Metadata, lang)
		if doc.Warning != "" {
			fmt.Fprintf(&body, "#text(fill: clr-error-fg, %s)\n", reportQuote(reportLabel("Source text is unavailable or some cited lines could not be resolved. The saved citations are retained above.", lang)))
		}
		for _, scoped := range d.Scoped {
			if scoped.InputRecordID == doc.ID {
				reportHeading(&body, 3, "Saved document match", lang)
				reportFields(&body, scoped, lang)
			}
		}
	}
	reportHeading(&body, 1, "Profile and request details", lang)
	reportHeading(&body, 2, "Request", lang)
	reportFields(&body, d.Request, lang)
	reportHeading(&body, 2, "Product profile (current)", lang)
	if d.Profile != nil {
		reportFields(&body, d.Profile, lang)
	}
	reportHeading(&body, 2, "Scope nodes (current)", lang)
	for _, node := range d.Nodes {
		reportHeading(&body, 3, node.Label, lang)
		reportFields(&body, node, lang)
	}

	title := fmt.Sprintf("Profile %d", d.Summary.ProfileID)
	if d.Profile != nil {
		title = d.Profile.Name
		if lang == "en" && d.Profile.NameEN != "" {
			title = d.Profile.NameEN
		} else if lang == "zh-cn" && d.Profile.NameCN != "" {
			title = d.Profile.NameCN
		}
	}
	templateLabels := map[string]string{"report-title": "Product Review Report", "page": "Page",
		"table-of-contents": "Table of Contents", "basic-information": "Basic Information",
		"document-title": "Product", "document-id": "Run", "document-date": "Created", "reviewer": "Reviewer",
		"review-date": "Review Date", "review-scope": "Review Scope", "review-results": "Review Results",
		"summary": "Summary", "statistics": "Statistics", "aspect": "Category", "findings": "Count",
		"grounding-references": "Source Documents", "supporting-references": "Supporting References",
		"no-references": "No references listed."}
	var b strings.Builder
	fmt.Fprintf(&b, "#import %s: *\n#document-review-report(\n", reportQuote(templatePath))
	typstLang := "en"
	if lang == "zh-cn" {
		typstLang = "zh"
	}
	fmt.Fprintf(&b, "lang: %s, font: (\"Linux Libertine\", \"PingFang SC\", \"Noto Sans CJK SC\"),\nlabels: (\n", reportQuote(typstLang))
	keys := make([]string, 0, len(templateLabels))
	for key := range templateLabels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&b, "%s: %s,\n", reportQuote(key), reportQuote(reportLabel(templateLabels[key], lang)))
	}
	fmt.Fprintf(&b, "),\ndoc-title: %s, doc-id: %s, doc-date: %s, review-date: %s, reviewer: %s, review-scope: %s,\n",
		reportQuote(title), reportQuote(fmt.Sprintf("%d · request %d · run %d", d.Run.ID, d.Run.RequestID, d.Run.RunNumber)),
		reportQuote(d.Run.CreatedAt.Format("2006-01-02 15:04 MST")), reportQuote(d.Summary.GeneratedAt.Format("2006-01-02 15:04 MST")),
		reportQuote(d.Request.Requester), reportQuote(fmt.Sprintf("Profile %d · v%d · %s", d.Summary.ProfileID, d.Summary.ProfileVersion, strings.Join(d.Summary.ArtifactTypes, ", "))))
	fmt.Fprintf(&b, "summary: [#text(%s) #parbreak() #text(%s)],\naspect-stats: (\n",
		reportQuote(reportLabel("All saved results are included, regardless of screen filters. Truncated results were excluded during the original run and cannot be reconstructed by this export.", lang)),
		reportQuote(reportLabel("Counts, coverage, scores, citations and match reasons are saved run data. Metric fields, source text, document metadata and the profile appendix are current records, not historical snapshots. Empty fields are omitted.", lang)))
	for _, stat := range []struct {
		label string
		n     int
	}{
		{"Results", d.Run.ResultCount}, {"Attributed results", d.Run.AttributedCount},
		{"Document-scope results", d.Run.DocumentScopeCount}, {"Documents in scope", d.Run.ScopedDocumentCount},
		{"Truncated results", d.Run.TruncatedCount}, {"Coverage gaps", len(d.Summary.Gaps)},
	} {
		fmt.Fprintf(&b, "(aspect: %s, count: %d),\n", reportQuote(reportLabel(stat.label, lang)), stat.n)
	}
	fmt.Fprintf(&b, "),\naspects: ([%s],),\ncontents-depth: 2,\ngrounding-refs: (\n", body.String())
	for _, doc := range d.Documents {
		fmt.Fprintf(&b, "(id: %s, title: %s, description: %s),\n", reportQuote(fmt.Sprintf("%d", doc.ID)),
			reportQuote(reportValue(doc.Metadata["title"])), reportQuote(reportValue(doc.Metadata["document_number"])))
	}
	b.WriteString("),\n)\n")
	return b.String()
}
