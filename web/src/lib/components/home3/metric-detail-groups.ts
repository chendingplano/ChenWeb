// Shared "Metric / Metadata / Context / Grounding / Reasoning" attribute-group
// builder for a single kb.metrics row — the canonical field set behind the
// Metric Detail Visualizer (shared-ui/metric-detail/, shown in the Metrics
// workspace's info panel) and the Product Review "Full Details" dialog
// (product-metric-review-view.svelte). Keeping this in one place means both
// views show the same fields for the same metric.
import type { KbMetricRecord, RawLine } from '$lib/services/kbService';
import { m as msg } from '$lib/paraglide/messages.js';
import { classifyMetricStatement } from '$lib/metric-statement-kind';
import { STATEMENT_KIND_LABEL } from '$lib/metric-statement-kind-labels';
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
export type AttrKind = 'text' | 'chips' | 'lines' | 'json';
export type LineEntry = { head: string; content: string; lineType: string };
export type JsonEntry = { key: string; value: string | null; depth: number };
export type AttrDef = {
	key: string;
	label: string;
	icon: unknown;
	kind: AttrKind;
	value: string; // formatted value for `text` kind, joined for chips/lines summary
	items: string[]; // for `chips` and `lines` kinds (joined head + content for lines)
	entries: LineEntry[]; // structured per-line entries for `lines` kind
	jsonEntries?: JsonEntry[];
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

function flattenJsonEntries(value: unknown, depth = 0, key = ''): JsonEntry[] {
	if (Array.isArray(value)) {
		if (value.length === 0) return [{ key, value: '[]', depth }];
		if (value.every((item) => item == null || typeof item !== 'object')) {
			return [{ key, value: JSON.stringify(value), depth }];
		}
		return [
			{ key, value: null, depth },
			...value.flatMap((item, index) => flattenJsonEntries(item, depth + 1, `[${index}]`))
		];
	}
	if (value != null && typeof value === 'object') {
		const entries = Object.entries(value as Record<string, unknown>);
		if (entries.length === 0) return [{ key, value: '{}', depth }];
		return [
			{ key, value: null, depth },
			...entries.flatMap(([childKey, childValue]) =>
				flattenJsonEntries(childValue, depth + 1, childKey)
			)
		];
	}
	return [{ key, value: value == null ? 'null' : String(value), depth }];
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

/** Display label of a frame resource role (agent, apparatus, …); an unknown role shows as-is. */
export function metricFrameRoleLabel(role: string): string {
	const labels: Record<string, () => string> = {
		agent: msg.metric_frame_role_agent,
		material: msg.metric_frame_role_material,
		apparatus: msg.metric_frame_role_apparatus,
		reagent: msg.metric_frame_role_reagent,
		medium: msg.metric_frame_role_medium,
		sample: msg.metric_frame_role_sample,
		device: msg.metric_frame_role_device
	};
	return labels[role]?.() ?? role;
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
	const frame = m.frame ?? {};
	const useItems = (frame.uses ?? []).map((r) => {
		const spec = (r.spec ?? []).map((s) => `${s.property} ${s.value}${s.unit ?? ''}`.trim()).join(', ');
		const name = spec ? `${r.name} (${spec})` : r.name;
		return msg.metric_frame_use_item({ role: metricFrameRoleLabel(r.role), name });
	});
	const methodItems = (frame.method ?? []).map((q) => (q.label ? `${q.label}：${q.value}` : q.value));
	const procedureText = [frame.procedure?.step, frame.procedure?.stage, frame.procedure?.after]
		.filter((v) => v != null && v.trim() !== '')
		.join(' · ');

	const metadata: AttrDef[] = [
		textAttr('metric_id', msg.metric_detail_id(), HashIcon, String(m.id), true),
		textAttr('input_record_id', msg.metric_detail_document_id(), HashIcon, String(m.input_record_id), true),
		...(m.run_id ? [textAttr('run_id', msg.metric_mgmt_run_id(), HashIcon, m.run_id, true)] : []),
		textAttr(
			'confidence',
			msg.metric_detail_confidence(),
			ActivityIcon,
			confidencePct(m.confidence),
			m.confidence != null
		),
		textAttr('desc', msg.metric_detail_desc(), FileTextIcon, fmt(m.metric_desc), has(m.metric_desc)),
		textAttr(
			'formula',
			msg.metric_detail_formula(),
			HashIcon,
			fmt(m.formula_or_definition),
			has(m.formula_or_definition)
		),
		textAttr(
			'explicit',
			msg.metric_detail_explicit(),
			CalendarIcon,
			m.is_explicit_metric == null ? '' : m.is_explicit_metric ? 'true' : 'false',
			m.is_explicit_metric != null
		),
		textAttr(
			'keyword_concept_id',
			msg.metric_detail_keyword_concept_id(),
			HashIcon,
			fmt(m.keyword_concept_id),
			has(m.keyword_concept_id)
		),
		textAttr(
			'metric_definition_term_id',
			msg.metric_detail_definition_term_id(),
			HashIcon,
			fmt(m.metric_definition_term_id),
			has(m.metric_definition_term_id)
		),
		textAttr(
			'value_range_type_error',
			msg.metric_detail_range_type_error(),
			HashIcon,
			fmt(m.value_range_type_error),
			has(m.value_range_type_error)
		)
	];

	const context: AttrDef[] = [
		textAttr(
			'document_title',
			msg.metric_detail_document_title(),
			FileIcon,
			fmt(m.document_title),
			has(m.document_title)
		),
		textAttr('document_doc_no', msg.metric_detail_doc_no(), HashIcon, fmt(m.document_doc_no), has(m.document_doc_no)),
		textAttr(
			'table_section',
			msg.metric_detail_section(),
			ListIcon,
			fmt(m.table_name_or_section),
			has(m.table_name_or_section)
		),
		textAttr('context', msg.metric_detail_context(), BookOpenIcon, fmt(m.metric_context), has(m.metric_context)),
		chipsAttr('keywords', msg.metric_detail_keywords(), TagIcon, kwItems, kwItems.join(', ')),
		textAttr(
			'search_document',
			msg.metric_mgmt_search_document(),
			FileTextIcon,
			fmt(m.search_document),
			has(m.search_document)
		)
	];

	const metric: AttrDef[] = [
		textAttr('name', msg.metric_detail_name(), TypeIcon, fmt(m.metric_name), has(m.metric_name)),
		textAttr('metric_artifact_id', msg.metric_detail_metric_id(), HashIcon, fmt(m.metric_id), has(m.metric_id)),
		textAttr('subject', msg.metric_detail_subject(), TypeIcon, fmt(m.metric_subject), has(m.metric_subject)),
		textAttr('condition', msg.metric_mgmt_condition(), MapPinIcon, fmt(m.condition), has(m.condition)),
		textAttr(
			'condition_en',
			msg.metric_mgmt_condition_en(),
			MapPinIcon,
			fmt(m.condition_en),
			has(m.condition_en)
		),
		chipsAttr('frame_uses', msg.metric_frame_uses(), TagIcon, useItems, useItems.join('; ')),
		chipsAttr('frame_method', msg.metric_frame_method(), TagIcon, methodItems, methodItems.join('; ')),
		textAttr('frame_procedure', msg.metric_frame_procedure(), ListIcon, procedureText, procedureText !== ''),
		textAttr('object_name', msg.metric_detail_object(), FileIcon, fmt(m.object_name), has(m.object_name)),
		textAttr(
			'frequency',
			msg.metric_detail_frequency(),
			CalendarIcon,
			fmt(m.measurement_frequency),
			has(m.measurement_frequency)
		),
		textAttr('value', msg.metric_detail_value(), TrendingUpIcon, fmt(m.metric_value), has(m.metric_value)),
		textAttr(
			'threshold',
			msg.metric_detail_threshold(),
			TrendingUpIcon,
			fmt(m.threshold_or_target),
			has(m.threshold_or_target)
		),
		textAttr('unit', msg.metric_detail_unit(), HashIcon, fmt(m.metric_unit), has(m.metric_unit)),
		textAttr('value_class', msg.metric_detail_class(), TagIcon, fmt(m.value_class), has(m.value_class)),
		textAttr(
			'statement_kind',
			msg.metric_statement_kind_label(),
			TagIcon,
			STATEMENT_KIND_LABEL[classifyMetricStatement(m).kind](),
			true
		),
		textAttr(
			'value_data_type',
			msg.metric_detail_data_type(),
			ListIcon,
			fmt(m.value_data_type),
			has(m.value_data_type)
		),
		textAttr(
			'value_range_type',
			msg.metric_detail_range_type(),
			TrendingUpIcon,
			fmt(m.value_range_type),
			has(m.value_range_type)
		),
		textAttr('location_type', msg.metric_detail_location(), MapPinIcon, fmt(m.location_type), has(m.location_type)),
		{
			key: 'frame_json',
			label: msg.metric_frame_json(),
			icon: ListIcon,
			kind: 'json',
			value: '',
			items: [],
			entries: [],
			jsonEntries: Object.entries(m.frame ?? {}).flatMap(([key, value]) =>
				flattenJsonEntries(value, 1, key)
			),
			count: Object.keys(m.frame ?? {}).length,
			hasValue: Object.keys(m.frame ?? {}).length > 0
		}
	];

	const reasoning: AttrDef[] = [chipsAttr('reasoning_tags', msg.metric_detail_tags(), TagIcon, tags, tags.join(', '))];

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
	const grounding: AttrDef[] = [linesAttr('source_line_spans', msg.metric_detail_lines(), FileTextIcon, groundingEntries)];

	return { metadata, context, metric, reasoning, grounding };
}
