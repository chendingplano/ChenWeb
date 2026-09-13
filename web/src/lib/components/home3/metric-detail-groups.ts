// Shared "Metric / Metadata / Context / Grounding / Reasoning" attribute-group
// builder for a single kb.metrics row — the canonical field set behind the
// Metrics workspace's info panel (metric-mgmt-view.svelte) and the Product
// Review "Full Details" dialog (product-metric-review-view.svelte). Keeping
// this in one place means both views show the same fields for the same metric.
import type { KbMetricRecord, RawLine } from '$lib/services/kbService';
import ActivityIcon from '@lucide/svelte/icons/activity';
import BookOpenIcon from '@lucide/svelte/icons/book-open';
import CalendarIcon from '@lucide/svelte/icons/calendar';
import FileIcon from '@lucide/svelte/icons/file';
import FileTextIcon from '@lucide/svelte/icons/file-text';
import HashIcon from '@lucide/svelte/icons/hash';
import ListIcon from '@lucide/svelte/icons/list';
import MapPinIcon from '@lucide/svelte/icons/map-pin';
import TagIcon from '@lucide/svelte/icons/tag';
import TrendingUpIcon from '@lucide/svelte/icons/trending-up';
import TypeIcon from '@lucide/svelte/icons/type';

export type NormalizedSpan = { page_number: number; line_number: number };
export type AttrKind = 'text' | 'chips' | 'lines';
export type LineEntry = { head: string; content: string; lineType: string };
export type AttrDef = {
	key: string;
	label: string;
	icon: unknown;
	kind: AttrKind;
	value: string; // formatted value for `text` kind, joined for chips/lines summary
	items: string[]; // for `chips` and `lines` kinds (joined head + content for lines)
	entries: LineEntry[]; // structured per-line entries for `lines` kind
	count: number; // 1 for scalars with a value, items.length for lists, 0 if empty
	hasValue: boolean;
};
export type MetricGroupAttrs = {
	metadata: AttrDef[];
	context: AttrDef[];
	metric: AttrDef[];
	reasoning: AttrDef[];
	grounding: AttrDef[];
};

function toPositiveInt(v: unknown): number | null {
	const n = typeof v === 'string' ? Number(v.trim()) : Number(v);
	if (!Number.isFinite(n)) return null;
	const i = Math.trunc(n);
	return i > 0 ? i : null;
}

/** Maps line_number → page_number, built from a document's loaded raw lines. */
export function buildLineNumToPage(rawLines: RawLine[]): Map<number, number> {
	const map = new Map<number, number>();
	for (const ln of rawLines) {
		if (!map.has(ln.line_number)) map.set(ln.line_number, ln.page_number);
	}
	return map;
}

// source_line_spans uses line-number spans only ("90", "98:99"). Page numbers
// are resolved via lineNumToPage (built from the owning document's raw lines).
export function normalizeMetricSpans(
	m: { source_line_spans?: unknown } | undefined,
	lineNumToPage: Map<number, number>
): NormalizedSpan[] {
	const raw = m?.source_line_spans;
	if (!Array.isArray(raw)) return [];
	const lineNums: number[] = [];
	for (const item of raw) {
		if (typeof item === 'string') {
			const s = item.trim();
			const mm = s.match(/^(\d+)\s*[:,-]\s*(\d+)$/);
			if (mm) {
				const start = parseInt(mm[1], 10);
				const end = parseInt(mm[2], 10);
				for (let n = start; n <= end && n <= start + 200; n++) lineNums.push(n);
			} else {
				const n = parseInt(s, 10);
				if (n > 0) lineNums.push(n);
			}
		} else if (typeof item === 'number' && item > 0) {
			lineNums.push(Math.trunc(item));
		} else if (item && typeof item === 'object') {
			const obj = item as Record<string, unknown>;
			const l = toPositiveInt(obj.line_number ?? obj.line ?? obj.line_no ?? obj.lineNo);
			if (l) lineNums.push(l);
		}
	}
	const out: NormalizedSpan[] = [];
	for (const lineNo of lineNums) {
		const pageNo = lineNumToPage.get(lineNo);
		if (pageNo) out.push({ page_number: pageNo, line_number: lineNo });
	}
	return out;
}

export function confidencePct(c?: number): string {
	if (c == null) return '—';
	return `${Math.round(c * 100)}%`;
}

export function buildMetricGroupAttrs(
	m: KbMetricRecord,
	spans: NormalizedSpan[],
	lineByKey: Map<string, RawLine>
): MetricGroupAttrs {
	const fmt = (v: unknown): string => (v == null || v === '' ? '' : String(v));
	const has = (v: unknown): boolean => v != null && v !== '';
	const textAttr = (
		key: string,
		label: string,
		icon: unknown,
		value: string,
		hasValue: boolean
	): AttrDef => ({
		key,
		label,
		icon,
		kind: 'text',
		value,
		items: [],
		entries: [],
		count: hasValue ? 1 : 0,
		hasValue
	});
	const chipsAttr = (
		key: string,
		label: string,
		icon: unknown,
		items: string[],
		value: string
	): AttrDef => ({
		key,
		label,
		icon,
		kind: 'chips',
		value,
		items,
		entries: [],
		count: items.length,
		hasValue: items.length > 0
	});
	const linesAttr = (
		key: string,
		label: string,
		icon: unknown,
		entries: LineEntry[]
	): AttrDef => {
		const items = entries.map((e) => (e.content ? `${e.head}: ${e.content}` : e.head));
		return {
			key,
			label,
			icon,
			kind: 'lines',
			value: items.join('\n'),
			items,
			entries,
			count: entries.length,
			hasValue: entries.length > 0
		};
	};

	const kwItems = (m.metric_keywords ?? []).filter((v) => typeof v === 'string' && v.trim() !== '');
	const tags = (m.reasoning_tags ?? []).filter((v) => typeof v === 'string' && v.trim() !== '');

	const metadata: AttrDef[] = [
		textAttr('metric_id', 'ID', HashIcon, String(m.id), true),
		textAttr('input_record_id', 'Document ID', HashIcon, String(m.input_record_id), true),
		textAttr(
			'confidence',
			'Confidence',
			ActivityIcon,
			confidencePct(m.confidence),
			m.confidence != null
		),
		textAttr('desc', 'Desc', FileTextIcon, fmt(m.metric_desc), has(m.metric_desc)),
		textAttr(
			'formula',
			'Formula',
			HashIcon,
			fmt(m.formula_or_definition),
			has(m.formula_or_definition)
		),
		textAttr(
			'explicit',
			'Explicit',
			CalendarIcon,
			m.is_explicit_metric == null ? '' : m.is_explicit_metric ? 'true' : 'false',
			m.is_explicit_metric != null
		),
		textAttr(
			'keyword_concept_id',
			'Keyword Concept ID',
			HashIcon,
			fmt(m.keyword_concept_id),
			has(m.keyword_concept_id)
		),
		textAttr(
			'metric_definition_term_id',
			'Definition Term ID',
			HashIcon,
			fmt(m.metric_definition_term_id),
			has(m.metric_definition_term_id)
		),
		textAttr(
			'value_range_type_error',
			'Range Type Error',
			HashIcon,
			fmt(m.value_range_type_error),
			has(m.value_range_type_error)
		)
	];

	const context: AttrDef[] = [
		textAttr(
			'document_title',
			'Document Title',
			FileIcon,
			fmt(m.document_title),
			has(m.document_title)
		),
		textAttr('document_doc_no', 'Doc No', HashIcon, fmt(m.document_doc_no), has(m.document_doc_no)),
		textAttr(
			'table_section',
			'Section',
			ListIcon,
			fmt(m.table_name_or_section),
			has(m.table_name_or_section)
		),
		textAttr('context', 'Context', BookOpenIcon, fmt(m.metric_context), has(m.metric_context)),
		chipsAttr('keywords', 'Keywords', TagIcon, kwItems, kwItems.join(', '))
	];

	const metric: AttrDef[] = [
		textAttr('name', 'Name', TypeIcon, fmt(m.metric_name), has(m.metric_name)),
		textAttr('metric_artifact_id', 'Metric ID', HashIcon, fmt(m.metric_id), has(m.metric_id)),
		textAttr('subject', 'Subject', TypeIcon, fmt(m.metric_subject), has(m.metric_subject)),
		textAttr('object_name', 'Object', FileIcon, fmt(m.object_name), has(m.object_name)),
		textAttr(
			'frequency',
			'Frequency',
			CalendarIcon,
			fmt(m.measurement_frequency),
			has(m.measurement_frequency)
		),
		textAttr('value', 'Value', TrendingUpIcon, fmt(m.metric_value), has(m.metric_value)),
		textAttr(
			'threshold',
			'Threshold',
			TrendingUpIcon,
			fmt(m.threshold_or_target),
			has(m.threshold_or_target)
		),
		textAttr('unit', 'Unit', HashIcon, fmt(m.metric_unit), has(m.metric_unit)),
		textAttr('value_class', 'Class', TagIcon, fmt(m.value_class), has(m.value_class)),
		textAttr(
			'value_data_type',
			'Data Type',
			ListIcon,
			fmt(m.value_data_type),
			has(m.value_data_type)
		),
		textAttr(
			'value_range_type',
			'Range Type',
			TrendingUpIcon,
			fmt(m.value_range_type),
			has(m.value_range_type)
		),
		textAttr('location_type', 'Location', MapPinIcon, fmt(m.location_type), has(m.location_type))
	];

	const reasoning: AttrDef[] = [chipsAttr('reasoning_tags', 'Tags', TagIcon, tags, tags.join(', '))];

	const groundingEntries: LineEntry[] = spans.flatMap((span) => {
		const rawLine = lineByKey.get(`${span.page_number}:${span.line_number}`);
		const content = rawLine?.content ?? '';
		const lineType = rawLine?.line_type ?? '';
		const head = `L${span.line_number} · P${span.page_number}`;
		const segs = content.split('\n').filter((s) => s.trim() !== '');
		if (segs.length <= 1) return [{ head, content, lineType }];
		return segs.map((seg, i) => ({
			head: i === 0 ? head : `${head} · ${i + 1}`,
			content: seg,
			lineType: i === 0 ? lineType : ''
		}));
	});
	const grounding: AttrDef[] = [linesAttr('source_line_spans', 'Lines', FileTextIcon, groundingEntries)];

	return { metadata, context, metric, reasoning, grounding };
}
