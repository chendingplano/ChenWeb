// API client + pure helpers for the "Review Metrics" admin page
// (System Admin -> LLM). See openspec/changes/llm-review-metrics for the design,
// and openspec/changes/metric-review-i18n-export for languages and export.

import { Marked } from 'marked';
import type { TableContextWindow } from './metric-table-context.js';

export type ReviewSeverity = 'high' | 'medium' | 'low';
export type NonMetricCategory = 'not_metric' | 'duplicate' | 'formula_input';

export type InputRecordSummary = {
	id: number;
	create_time?: string;
	title?: string;
	doc_no?: string;
	file_name?: string;
};

export type MetricReviewTally = {
	stored: number;
	kept: number;
	not_metric: number;
	duplicate: number;
	formula_input: number;
	missed: number;
};

export type MissedMetric = {
	lines: string;
	source_line_spans?: string[];
	/** Table rows the review cited (same shape as kb.metrics.source_table_rows). */
	source_table_rows?: { line: number; rows: string[] }[];
	/** Built by the server on GET from source_table_rows: header + cited rows ± 1. */
	table_context?: TableContextWindow[];
	name: string;
	value: string;
	unit: string;
	reason: string;
	severity: ReviewSeverity;
};

export type NonMetricEntry = {
	metric_ids: string[];
	category: NonMetricCategory;
	duplicate_of?: string;
	reason: string;
};

export type AttributeIssue = {
	metric_ids: string[];
	field: string;
	stored: string;
	suggested: string;
	reason: string;
	severity: ReviewSeverity;
};

export type MetricSnapshot = {
	metric_id: string;
	name: string;
	value?: string;
	unit?: string;
	lines?: string;
	source_line_spans?: string[];
};

/** Expand the extractor's one-based source spans into raw line numbers. */
export function reviewLineNumbers(spans: string[]): number[] {
	const numbers = new Set<number>();
	for (const span of spans) {
		const match = String(span).trim().match(/^(?:L)?(\d+)(?:\s*[:\-]\s*(\d+))?$/i);
		if (!match) continue;
		const start = Number(match[1]);
		const end = match[2] ? Number(match[2]) : start;
		if (start < 1 || end < start || end - start > 1000) continue;
		for (let line = start; line <= end; line++) numbers.add(line);
	}
	return [...numbers];
}

export type MetricReviewReport = {
	summary: string;
	tally: MetricReviewTally;
	missed_metrics: MissedMetric[];
	non_metrics: NonMetricEntry[];
	attribute_issues: AttributeIssue[];
	recommendations: string[];
	metrics: MetricSnapshot[];
};

export type MetricReview = {
	id: number;
	input_record_id: number;
	/** Language of the report prose: 'en' | 'zh-cn'. */
	lang: string;
	status: 'running' | 'done' | 'failed';
	report?: MetricReviewReport;
	error_msg?: string;
	model_name?: string;
	prompt_name?: string;
	metrics_count: number;
	created_by?: string;
	created_at: string;
	finished_at?: string;
	/** Set when this review is a translation of another review. */
	translated_from_id?: number;
};

type MetricReviewResponse = {
	status: boolean;
	review: MetricReview | null;
	started?: boolean;
	/** GET only: other languages that have a finished review. */
	other_langs?: string[];
};

/** A purely numeric query searches by record ID; anything else by title. */
export function buildInputSearchQuery(query: string, hasGoldMetrics = false): string {
	const q = query.trim();
	const params = new URLSearchParams({ page: '1', page_size: '50' });
	if (/^\d+$/.test(q)) params.set('record_id', q);
	else if (q) params.set('title', q);
	if (hasGoldMetrics) params.set('has_gold_metrics', 'true');
	return params.toString();
}

const SEVERITY_RANK: Record<string, number> = { high: 0, medium: 1, low: 2 };

/** Highest severity first; stable within a severity. */
export function sortBySeverity<T extends { severity: string }>(items: T[]): T[] {
	return [...items].sort((a, b) => (SEVERITY_RANK[a.severity] ?? 1) - (SEVERITY_RANK[b.severity] ?? 1));
}

export const NON_METRIC_CATEGORY_ORDER: NonMetricCategory[] = ['not_metric', 'duplicate', 'formula_input'];

export const NON_METRIC_CATEGORY_LABEL: Record<NonMetricCategory, string> = {
	not_metric: 'Not a metric',
	duplicate: 'Duplicate',
	formula_input: 'Formula input'
};

/** Groups non-metric entries by category in NON_METRIC_CATEGORY_ORDER, omitting empty groups. */
export function groupNonMetrics(
	entries: NonMetricEntry[]
): { category: NonMetricCategory; entries: NonMetricEntry[] }[] {
	return NON_METRIC_CATEGORY_ORDER.map((category) => ({
		category,
		entries: entries.filter((e) => e.category === category)
	})).filter((g) => g.entries.length > 0);
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(path, { credentials: 'same-origin', ...init });
	const text = await res.text();
	let parsed: unknown = null;
	if (text) {
		try {
			parsed = JSON.parse(text);
		} catch {
			parsed = null;
		}
	}
	if (!res.ok) {
		const msg =
			parsed && typeof parsed === 'object' && 'error_msg' in parsed
				? String((parsed as { error_msg: unknown }).error_msg)
				: `HTTP ${res.status}`;
		throw new Error(msg);
	}
	return parsed as T;
}

export async function searchInputs(query: string, hasGoldMetrics = false): Promise<InputRecordSummary[]> {
	const res = await req<{ results?: InputRecordSummary[] }>(
		`/api/v1/kb/inputs?${buildInputSearchQuery(query, hasGoldMetrics)}`
	);
	return res.results ?? [];
}

/** The record's newest review in `lang`, plus the other languages that have one. */
export async function getMetricReview(
	recordId: number,
	lang: string
): Promise<{ review: MetricReview | null; otherLangs: string[] }> {
	const res = await req<MetricReviewResponse>(
		`/api/v1/kb/metric-reviews/${recordId}?lang=${encodeURIComponent(lang)}`
	);
	return { review: res.review, otherLangs: res.other_langs ?? [] };
}

/** LLM models (.models.toml keys with model_type "llm") and the server default. */
export async function listMetricReviewModels(): Promise<{ models: string[]; defaultModel: string }> {
	const res = await req<{ models?: string[]; default?: string }>('/api/v1/kb/metric-reviews/models');
	return { models: res.models ?? [], defaultModel: res.default ?? '' };
}

/** `model` is a .models.toml key; empty uses the server default. */
export async function startMetricReview(
	recordId: number,
	force: boolean,
	lang: string,
	model: string
): Promise<MetricReviewResponse> {
	return req<MetricReviewResponse>(`/api/v1/kb/metric-reviews/${recordId}`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ force, lang, model })
	});
}

/** Translates the record's newest finished review in another language into `lang`. */
export async function translateMetricReview(recordId: number, lang: string): Promise<MetricReviewResponse> {
	return req<MetricReviewResponse>(`/api/v1/kb/metric-reviews/${recordId}/translate`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ lang })
	});
}

// --- Export ---

/** Localised text used by the Markdown/PDF export (built by the page from paraglide messages). */
export type ReviewExportLabels = {
	title: string;
	record: string;
	reviewed: string;
	model: string;
	prompt: string;
	/** Already formatted, e.g. "Translated from review #4"; empty when not a translation. */
	translatedFrom: string;
	tally: Record<keyof MetricReviewTally, string>;
	missed: string;
	/** Attribute labels inside each metric sub-section. */
	lines: string;
	grounding: string;
	metricId: string;
	description: string;
	context: string;
	unit: string;
	value: string;
	nonMetrics: string;
	attributes: string;
	recommendations: string;
	none: string;
	duplicateOf: string;
	empty: string;
	category: Record<NonMetricCategory, string>;
	severity: Record<ReviewSeverity, string>;
};

export function reviewExportFilename(recordId: number, lang: string, ext: string): string {
	return `review-${recordId}-${lang}.${ext}`;
}

/** A kb.metrics row as returned by GET /kb/metrics (only the fields the export shows). */
export type StoredMetric = {
	metric_id?: string | null;
	metric_name?: string;
	metric_name_en?: string;
	metric_desc?: string;
	metric_desc_en?: string;
	metric_context?: string;
	metric_context_en?: string;
	metric_unit?: string;
	metric_value?: string;
	source_line_spans?: (string | number | { line_number: number })[];
	table_context?: TableContextWindow[];
};

/** What the export quotes from: the record's line file and its current kb.metrics rows. */
export type ReviewExportSource = {
	lines?: { line_number: number; content: string }[];
	metrics?: StoredMetric[];
};

/** Line spans as "116" / "12:15" strings; table-row suffixes ("116#r1") are dropped. */
function spanStrings(spans: StoredMetric['source_line_spans']): string[] {
	return (spans ?? [])
		.map((x) => (typeof x === 'object' && x ? String(x.line_number) : String(x).replace(/#.*$/, '').trim()))
		.filter(Boolean);
}

/** Renders a done review as Markdown in the review's language (the labels' language). */
export function buildReviewMarkdown(
	rec: InputRecordSummary,
	review: MetricReview,
	L: ReviewExportLabels,
	fmtTime: (s?: string) => string,
	source: ReviewExportSource = {}
): string {
	const r = review.report;
	if (!r) return '';
	const en = review.lang === 'en';
	const stored = new Map((source.metrics ?? []).filter((m) => m.metric_id).map((m) => [m.metric_id as string, m]));
	const lineText = new Map((source.lines ?? []).map((l) => [l.line_number, l.content]));
	const flat = (s?: string | null) => (s ?? '').replace(/\s+/g, ' ').trim();
	const pick = (a?: string, b?: string) => flat(en ? b || a : a || b);
	const field = (label: string, v: string) => `- **${label}**: ${v || L.empty}`;
	const snap = new Map(r.metrics.map((m) => [m.metric_id, m]));
	const ids = (list: string[]) =>
		list
			.map((id) => {
				const name = snap.get(id)?.name;
				return name ? `\`${id}\` ${name}` : `\`${id}\``;
			})
			.join(', ');
	const sev = (s: ReviewSeverity) => `**[${L.severity[s] ?? s}]**`;
	const out: string[] = [];

	out.push(`# ${L.title}: ${rec.title || rec.file_name || ''}`.trimEnd(), '');
	out.push(`- ${L.record}: #${rec.id}${rec.doc_no ? ` · ${rec.doc_no}` : ''}`);
	out.push(`- ${L.reviewed}: ${fmtTime(review.finished_at || review.created_at)}${review.created_by ? ` · ${review.created_by}` : ''}`);
	out.push(`- ${L.model}: ${review.model_name ?? ''} · ${L.prompt}: ${review.prompt_name ?? ''}`);
	if (L.translatedFrom) out.push(`- ${L.translatedFrom}`);
	out.push('', r.summary, '');

	const keys: (keyof MetricReviewTally)[] = ['stored', 'kept', 'not_metric', 'duplicate', 'formula_input', 'missed'];
	out.push(`| ${keys.map((k) => L.tally[k]).join(' | ')} |`);
	out.push(`|${keys.map(() => '---:').join('|')}|`);
	out.push(`| ${keys.map((k) => r.tally[k]).join(' | ')} |`, '');

	out.push(`## ${L.missed} (${r.missed_metrics.length})`, '');
	if (r.missed_metrics.length === 0) out.push(L.none, '');
	// "Lines" and "Grounding" list items for a metric's line spans. A table line
	// is shown as a table (the metric's table_context window, with its cited rows
	// in bold, when it has one, else the whole table); any other line as a quote.
	const linesAndGrounding = (spans: string[], windows: TableContextWindow[] = []) => {
		const numbers = reviewLineNumbers(spans).sort((a, b) => a - b);
		out.push(field(L.lines, numbers.length ? spans.map((x) => `L${String(x).trim().replace(/^L/i, '')}`).join(', ') : ''));
		const grounding = numbers.filter((n) => lineText.has(n));
		out.push(`- **${L.grounding}**:${grounding.length ? '' : ` ${L.empty}`}`, '');
		for (const n of grounding) {
			const content = lineText.get(n) ?? '';
			const w = windows.find((x) => x.line === n && x.rows?.length);
			const table = w ? tableContextHtml(w) : isTableLine(content) ? sanitizeTableHtml(content) : '';
			if (table) out.push(`  **L${n}**`, '', `  ${table}`, '');
			else out.push(`  > **L${n}** ${groundingText(content)}`, '');
		}
	};
	// A stored metric's fields; falls back to the review's snapshot when the
	// kb.metrics row is gone (the metrics were re-extracted since the review).
	const storedMetric = (id: string, heading: string, extra: string[] = []) => {
		const m = stored.get(id);
		const s = snap.get(id);
		out.push(`${heading} ${(m ? pick(m.metric_name, m.metric_name_en) : '') || flat(s?.name) || id}`, '');
		out.push(field(L.metricId, `\`${id}\``), ...extra);
		out.push(field(L.description, m ? pick(m.metric_desc, m.metric_desc_en) : ''));
		out.push(field(L.context, m ? pick(m.metric_context, m.metric_context_en) : ''));
		out.push(field(L.unit, flat(m ? m.metric_unit : s?.unit)));
		out.push(field(L.value, flat(m ? m.metric_value : s?.value)));
		const spans = m ? spanStrings(m.source_line_spans) : s?.source_line_spans?.length ? s.source_line_spans : s?.lines ? s.lines.split(',') : [];
		linesAndGrounding(spans, m?.table_context);
	};

	for (const m of sortBySeverity(r.missed_metrics)) {
		const val = [m.value, m.unit].filter(Boolean).join(' ');
		out.push(`### ${sev(m.severity)} ${m.name}${val ? ` — ${val}` : ''}`, '');
		linesAndGrounding(m.source_line_spans?.length ? m.source_line_spans : m.lines ? m.lines.split(',') : [], m.table_context);
		if (m.reason) out.push(m.reason, '');
	}

	const removed = r.tally.not_metric + r.tally.duplicate + r.tally.formula_input;
	out.push(`## ${L.nonMetrics} (${removed})`, '');
	if (r.non_metrics.length === 0) out.push(L.none, '');
	for (const g of groupNonMetrics(r.non_metrics)) {
		out.push(`### ${L.category[g.category]}`, '');
		for (const e of g.entries) {
			for (const id of e.metric_ids) {
				storedMetric(id, '####', e.duplicate_of ? [field(L.duplicateOf, ids([e.duplicate_of]))] : []);
				if (e.reason) out.push(e.reason, '');
			}
		}
	}

	out.push(`## ${L.attributes} (${r.attribute_issues.length})`, '');
	if (r.attribute_issues.length === 0) out.push(L.none, '');
	for (const a of sortBySeverity(r.attribute_issues)) {
		out.push(`### ${sev(a.severity)} \`${a.field}\``, '');
		if (a.stored || a.suggested) out.push(`${a.stored || L.empty} → ${a.suggested || L.empty}`, '');
		if (a.reason) out.push(a.reason, '');
		for (const id of a.metric_ids) storedMetric(id, '####');
		out.push('');
	}

	if (r.recommendations.length > 0) {
		out.push(`## ${L.recommendations}`, '');
		r.recommendations.forEach((x, i) => out.push(`${i + 1}. ${x}`));
		out.push('');
	}
	return out.join('\n');
}

const TABLE_TAG = /<(\/?)(table|thead|tbody|tr|td|th|strong)\b([^>]*)>/gi;

function isTableLine(content: string): boolean {
	return /<table[\s>]/i.test(content);
}

function decodeEntities(s: string): string {
	return s
		.replace(/&lt;/g, '<')
		.replace(/&gt;/g, '>')
		.replace(/&quot;/g, '"')
		.replace(/&#39;/g, "'")
		.replace(/&nbsp;/g, ' ')
		.replace(/&amp;/g, '&');
}

/**
 * Rebuilds a MinerU table line keeping only table/thead/tbody/tr/td/th/strong tags and
 * numeric rowspan/colspan; every other tag is dropped and all text is escaped.
 * The result is a single line, and sanitizing it again returns it unchanged.
 */
export function sanitizeTableHtml(html: string): string {
	const out: string[] = [];
	const text = (t: string) => {
		const clean = decodeEntities(t.replace(/<[^>]*>/g, '')).replace(/\s+/g, ' ');
		if (clean.trim()) out.push(escapeHtml(clean));
	};
	let last = 0;
	for (const tag of String(html).matchAll(TABLE_TAG)) {
		text(html.slice(last, tag.index));
		last = tag.index + tag[0].length;
		const [, close, name, attrs] = tag;
		const lower = name.toLowerCase();
		if (close) {
			out.push(`</${lower}>`);
			continue;
		}
		let keep = '';
		if (lower === 'td' || lower === 'th') {
			for (const a of attrs.matchAll(/\b(rowspan|colspan)\s*=\s*["']?(\d{1,3})["']?/gi)) keep += ` ${a[1].toLowerCase()}="${a[2]}"`;
		}
		out.push(`<${lower}${keep}>`);
	}
	text(html.slice(last));
	return out.join('');
}

/**
 * A stored metric's table_context window (header rows, its matched rows in
 * bold, one neighbor row either side) as table HTML in sanitizeTableHtml form.
 */
export function tableContextHtml(w: TableContextWindow): string {
	const width = Math.max(1, ...w.rows.map((r) => r.cells.length));
	const rows = w.rows.map((r) => {
		const tag = r.header ? 'th' : 'td';
		const cell = (c: string, span = '') => {
			const t = escapeHtml(flatCell(c));
			return `<${tag}${span}>${r.matched && t ? `<strong>${t}</strong>` : t}</${tag}>`;
		};
		const cells = r.full_width ? cell(r.cells.join(' '), width > 1 ? ` colspan="${width}"` : '') : r.cells.map((c) => cell(c)).join('');
		return `<tr>${cells}</tr>`;
	});
	return sanitizeTableHtml(`<table>${rows.join('')}</table>`);
}

function flatCell(s: string): string {
	return String(s ?? '').replace(/\s+/g, ' ').trim();
}

/** One source line as a single line of plain text. */
export function groundingText(content: string): string {
	return String(content).replace(/\s+/g, ' ').trim();
}

function escapeHtml(s: string): string {
	return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

// Report text comes from an LLM, so raw HTML in it is shown as text, never rendered.
const safeMarked = new Marked({
	renderer: {
		html({ text }) {
			// Grounding tables are emitted pre-sanitized (sanitizeTableHtml); only
			// HTML already in that form is rendered.
			const t = text.trim();
			if (isTableLine(t) && sanitizeTableHtml(t) === t) return t;
			return escapeHtml(text);
		}
	}
});

/** A standalone, print-styled HTML page of the Markdown export (for Export PDF). */
export function buildReviewPrintHtml(markdown: string, title: string, lang: string): string {
	const body = safeMarked.parse(markdown, { async: false }) as string;
	return `<!doctype html><html lang="${escapeHtml(lang)}"><head><meta charset="utf-8"><title>${escapeHtml(title)}</title>
<style>
@page { margin: 16mm; }
body { font-family: -apple-system, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans CJK SC", sans-serif; color: #111; font-size: 11pt; line-height: 1.5; max-width: 180mm; margin: 0 auto; }
h1 { font-size: 16pt; margin: 0 0 8pt; } h2 { font-size: 13pt; margin: 16pt 0 6pt; border-bottom: 1px solid #ddd; padding-bottom: 2pt; } h3 { font-size: 11pt; margin: 10pt 0 4pt; color: #444; break-after: avoid; } h4 { font-size: 10.5pt; margin: 8pt 0 3pt; color: #555; break-after: avoid; }
ul, ol { padding-left: 18pt; } li { margin: 3pt 0; break-inside: avoid; }
code { font-family: ui-monospace, Menlo, monospace; font-size: 9.5pt; background: #f2f2f2; padding: 0 2pt; border-radius: 2pt; }
blockquote { margin: 4pt 0 6pt; padding: 2pt 8pt; border-left: 3px solid #ccc; color: #333; background: #fafafa; } blockquote p { margin: 2pt 0; }
table { border-collapse: collapse; margin: 8pt 0; } th, td { border: 1px solid #ccc; padding: 3pt 8pt; text-align: right; }
li table { font-size: 9pt; margin: 4pt 0 6pt; } li th, li td { text-align: left; vertical-align: top; padding: 2pt 5pt; } li tr { break-inside: avoid; } li:has(table) { break-inside: auto; }
</style></head><body>${body}</body></html>`;
}
