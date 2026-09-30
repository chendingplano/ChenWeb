import test from 'node:test';
import assert from 'node:assert/strict';

import { buildInputSearchQuery, groupNonMetrics, reviewLineNumbers, sortBySeverity, type NonMetricEntry } from './metric-review-client.js';

test('review source spans expand ranges and ignore invalid spans', () => {
	assert.deepEqual(reviewLineNumbers(['L12', '13:15', '14', '0', '17:16', 'text']), [12, 13, 14, 15]);
});

test('numeric query searches by record_id', () => {
	const p = new URLSearchParams(buildInputSearchQuery(' 416 '));
	assert.equal(p.get('record_id'), '416');
	assert.equal(p.get('title'), null);
});

test('text query searches by title', () => {
	const p = new URLSearchParams(buildInputSearchQuery('垃圾分类'));
	assert.equal(p.get('title'), '垃圾分类');
	assert.equal(p.get('record_id'), null);
});

test('blank query sends neither filter', () => {
	const p = new URLSearchParams(buildInputSearchQuery('  '));
	assert.equal(p.get('title'), null);
	assert.equal(p.get('record_id'), null);
});

test('sortBySeverity puts high first and keeps order within a severity', () => {
	const out = sortBySeverity([
		{ id: 1, severity: 'low' },
		{ id: 2, severity: 'high' },
		{ id: 3, severity: 'medium' },
		{ id: 4, severity: 'high' }
	]);
	assert.deepEqual(
		out.map((x) => x.id),
		[2, 4, 3, 1]
	);
});

test('groupNonMetrics orders categories and omits empty groups', () => {
	const entries: NonMetricEntry[] = [
		{ metric_ids: ['a'], category: 'formula_input', reason: '' },
		{ metric_ids: ['b'], category: 'not_metric', reason: '' }
	];
	const groups = groupNonMetrics(entries);
	assert.deepEqual(
		groups.map((g) => g.category),
		['not_metric', 'formula_input']
	);
});

import {
	buildReviewMarkdown,
	buildReviewPrintHtml,
	reviewExportFilename,
	type MetricReview,
	type ReviewExportLabels
} from './metric-review-client.js';

const LABELS: ReviewExportLabels = {
	title: '指标审查',
	record: '记录',
	reviewed: '审查时间',
	model: '模型',
	prompt: '提示词',
	translatedFrom: '译自审查 #4',
	tally: { stored: '已存储', kept: '保留', not_metric: '非指标', duplicate: '重复', formula_input: '公式输入', missed: '遗漏' },
	missed: '遗漏的指标',
	lines: '行号',
	grounding: '原文依据',
	metricId: '指标 ID',
	description: '描述',
	context: '上下文',
	unit: '单位',
	value: '数值',
	nonMetrics: '不应作为指标的行',
	attributes: '属性问题',
	recommendations: '建议',
	none: '无。',
	duplicateOf: '重复于',
	empty: '（空）',
	category: { not_metric: '非指标', duplicate: '重复', formula_input: '公式输入' },
	severity: { high: '高', medium: '中', low: '低' }
};

const REVIEW: MetricReview = {
	id: 5,
	input_record_id: 416,
	lang: 'zh-cn',
	status: 'done',
	metrics_count: 3,
	created_at: '2026-09-29T10:00:00Z',
	model_name: 'gpt-6-luna',
	prompt_name: 'prompt-translate-metric-review-v1.md',
	translated_from_id: 4,
	report: {
		summary: '提取总体良好。',
		tally: { stored: 3, kept: 1, not_metric: 1, duplicate: 1, formula_input: 0, missed: 1 },
		missed_metrics: [{ lines: '153', name: '浸提用聚乙烯瓶容积', value: '500', unit: 'mL', reason: '未覆盖', severity: 'medium' }],
		non_metrics: [
			{ metric_ids: ['416_mtc_2'], category: 'not_metric', reason: '适用范围描述' },
			{ metric_ids: ['416_mtc_3'], category: 'duplicate', duplicate_of: '416_mtc_1', reason: '重复' }
		],
		attribute_issues: [
			{ metric_ids: ['416_mtc_1'], field: 'value_range_type', stored: 'range', suggested: 'lower_bound', reason: '不小于', severity: 'high' }
		],
		recommendations: ['<script>alert(1)</script> 改进'],
		metrics: [
			{ metric_id: '416_mtc_1', name: '种子发芽指数' },
			{ metric_id: '416_mtc_2', name: '人口密度' },
			{ metric_id: '416_mtc_3', name: '发芽指数' }
		]
	}
};

test('export filename carries record and language', () => {
	assert.equal(reviewExportFilename(416, 'zh-cn', 'md'), 'review-416-zh-cn.md');
});

test('markdown export contains every section in the labels language', () => {
	const md = buildReviewMarkdown({ id: 416, title: '农村生活垃圾分类处理规范' }, REVIEW, LABELS, () => 'T');
	for (const want of [
		'# 指标审查: 农村生活垃圾分类处理规范',
		'- 记录: #416',
		'- 译自审查 #4',
		'提取总体良好。',
		'| 已存储 | 保留 | 非指标 | 重复 | 公式输入 | 遗漏 |',
		'| 3 | 1 | 1 | 1 | 0 | 1 |',
		'## 遗漏的指标 (1)',
		'### **[中]** 浸提用聚乙烯瓶容积 — 500 mL',
		'- **行号**: L153',
		'- **原文依据**: （空）',
		'## 不应作为指标的行 (2)',
		'#### 发芽指数\n\n- **指标 ID**: `416_mtc_3`\n- **重复于**: `416_mtc_1` 种子发芽指数',
		'### **[高]** `value_range_type`\n\nrange → lower_bound\n\n不小于\n\n#### 种子发芽指数',
		'## 建议'
	]) {
		assert.ok(md.includes(want), `missing ${JSON.stringify(want)}\n---\n${md}`);
	}
});

test('markdown export of a review without report is empty', () => {
	assert.equal(buildReviewMarkdown({ id: 1 }, { ...REVIEW, report: undefined }, LABELS, () => ''), '');
});

test('print html escapes raw HTML from the report', () => {
	const md = buildReviewMarkdown({ id: 416 }, REVIEW, LABELS, () => 'T');
	const html = buildReviewPrintHtml(md, 'review-416-zh-cn', 'zh-cn');
	assert.ok(!html.includes('<script>alert'), 'raw script tag must not survive');
	assert.ok(html.includes('&lt;script&gt;'));
	assert.ok(html.includes('<h2>遗漏的指标 (1)</h2>'));
	assert.ok(html.includes('<title>review-416-zh-cn</title>'));
});

test('missed metric grounding quotes text lines and renders table lines as tables', () => {
	const md = buildReviewMarkdown({ id: 416 }, { ...REVIEW, report: { ...REVIEW.report!, missed_metrics: [{ ...REVIEW.report!.missed_metrics[0], lines: '152:153' }] } }, LABELS, () => 'T', {
		lines: [
			{ line_number: 151, content: 'unrelated' },
			{ line_number: 152, content: '取 500 mL 聚乙烯瓶' },
			{ line_number: 153, content: '<table><tr><td rowspan="2" style="x" onclick="evil()">容积</td><td>500 &amp; mL<script>x</script></td></tr><tr><td>b</td></tr></table>' }
		]
	});
	assert.ok(md.includes('  > **L152** 取 500 mL 聚乙烯瓶'), md);
	assert.ok(md.includes('  <table><tr><td rowspan="2">容积</td><td>500 &amp; mLx</td></tr><tr><td>b</td></tr></table>'), md);
	assert.ok(!md.includes('unrelated'));
	const html = buildReviewPrintHtml(md, 't', 'zh-cn');
	assert.ok(html.includes('<h3><strong>[中]</strong> 浸提用聚乙烯瓶容积 — 500 mL</h3>'), html);
	assert.ok(html.includes('<blockquote>'));
	assert.ok(html.includes('<table><tr><td rowspan="2">容积</td>'), html);
	assert.ok(!html.includes('onclick') && !html.includes('<script'));
});

test('print html escapes an LLM table that is not in sanitized form', () => {
	const html = buildReviewPrintHtml('<table><tr><td onclick="x()">a</td></tr></table>', 't', 'en');
	assert.ok(!html.includes('<td onclick'));
	assert.ok(html.includes('&lt;table&gt;'));
});

test('stored metrics show kb.metrics fields in the review language and table_context grounding', () => {
	const metrics = [
		{
			metric_id: '416_mtc_1',
			metric_name: '种子发芽指数',
			metric_name_en: 'Seed germination index',
			metric_desc: '描述',
			metric_desc_en: 'Germination\n index',
			metric_context: '',
			metric_context_en: '',
			metric_unit: '%',
			metric_value: '70',
			source_line_spans: ['116#r3', { line_number: 158 }],
			table_context: [
				{
					line: 116,
					columns: [],
					rows: [
						{ id: '116#h0', cells: ['序号', '模式'], header: true },
						{ id: '116#r3', cells: ['1', '厌氧<产沼>'], matched: true },
						{ id: '116#r4', cells: ['注：见 CJJ 52'], full_width: true }
					]
				}
			]
		}
	];
	const lines = [
		{ line_number: 116, content: '<table><tr><td>whole table</td></tr></table>' },
		{ line_number: 158, content: '指数大于100%' }
	];
	const md = buildReviewMarkdown({ id: 416 }, { ...REVIEW, lang: 'en' }, LABELS, () => 'T', { lines, metrics });
	for (const want of [
		'#### Seed germination index\n\n- **指标 ID**: `416_mtc_1`\n- **描述**: Germination index\n- **上下文**: （空）',
		'- **单位**: %\n- **数值**: 70\n- **行号**: L116, L158',
		'  <table><tr><th>序号</th><th>模式</th></tr><tr><td><strong>1</strong></td><td><strong>厌氧&lt;产沼&gt;</strong></td></tr><tr><td colspan="2">注：见 CJJ 52</td></tr></table>',
		'  > **L158** 指数大于100%'
	]) {
		assert.ok(md.includes(want), `missing ${JSON.stringify(want)}\n---\n${md}`);
	}
	assert.ok(!md.includes('whole table'), 'table_context window replaces the whole table');
	const html = buildReviewPrintHtml(md, 't', 'en');
	assert.ok(html.includes('<tr><td><strong>1</strong></td>'), html);
	// A metric with no kb.metrics row falls back to the review snapshot.
	assert.ok(md.includes('#### 人口密度\n\n- **指标 ID**: `416_mtc_2`'), md);
});
