// Turns one kb.metrics row into the titled field groups of the metric detail
// visualizer. The field set itself comes from home3/metric-detail-groups.ts;
// this file only converts it to name-value fields.
// See KnowledgeStore/doc-repo/modules/module-metric-detail-visualizer.md.
import type { KbMetricRecord, RawLine } from '$lib/services/kbService';
import { m } from '$lib/paraglide/messages.js';
import {
	buildLineNumToPage,
	buildMetricGroupAttrs,
	normalizeMetricSpans,
	type AttrDef,
	type MetricGroupAttrs
} from '$lib/components/home3/metric-detail-groups';
import type { NameValueInput, NameValueItem } from '../name-value/name-value';

export type MetricDetailGroupKey = keyof MetricGroupAttrs;

export type MetricDetailGroup = {
	key: MetricDetailGroupKey;
	label: string;
	filled: number; // fields with a value
	total: number;
	fields: NameValueInput[];
};

// Identifier fields, shown in a monospace face.
const MONO_KEYS = new Set([
	'metric_id',
	'input_record_id',
	'run_id',
	'metric_artifact_id',
	'keyword_concept_id',
	'metric_definition_term_id'
]);

// One attribute → one name-value field. A field without a value keeps an empty
// value list, so the visualizer can show it as empty.
export function attrToField(a: AttrDef): NameValueInput {
	let values: NameValueItem[] = [];
	if (a.hasValue) {
		if (a.kind === 'chips') {
			values = a.items.map((text) => ({ text }));
		} else if (a.kind === 'json') {
			// Top-level keys arrive at depth 1; indent each level by two spaces.
			values = (a.jsonEntries ?? []).map((e) => ({
				text: '  '.repeat(Math.max(0, e.depth - 1)) + (e.value === null ? e.key : `${e.key}: ${e.value}`),
				mono: true
			}));
		} else if (a.kind === 'lines') {
			values = a.entries.map((e) => {
				const head = e.lineType ? `${e.head} (${e.lineType})` : e.head;
				return { text: e.content ? `${head}\n${e.content}` : head };
			});
		} else {
			values = [{ text: a.value, mono: MONO_KEYS.has(a.key) }];
		}
	}
	return { key: a.key, label: a.label, values };
}

// rawLines are the lines of the metric's document; Grounding shows the ones the
// metric cites. With no lines, Grounding is empty.
export function buildMetricDetailGroups(metric: KbMetricRecord, rawLines: RawLine[]): MetricDetailGroup[] {
	const lineByKey = new Map(rawLines.map((ln) => [`${ln.page_number}:${ln.line_number}`, ln]));
	const spans = normalizeMetricSpans(metric, buildLineNumToPage(rawLines));
	const attrs = buildMetricGroupAttrs(metric, spans, lineByKey);
	const order: [MetricDetailGroupKey, string][] = [
		['metric', m.metric_detail_group_metric()],
		['metadata', m.metric_detail_group_metadata()],
		['context', m.metric_detail_group_context()],
		['grounding', m.metric_detail_group_grounding()],
		['reasoning', m.metric_detail_group_reasoning()]
	];
	return order.map(([key, label]) => ({
		key,
		label,
		filled: attrs[key].filter((a) => a.hasValue).length,
		total: attrs[key].length,
		fields: attrs[key].map(attrToField)
	}));
}

// Heading of the panel: the metric's name (or subject), with its English name below.
export function metricTitle(metric: KbMetricRecord): { name: string; subname: string } {
	return {
		name:
			metric.metric_name?.trim() ||
			metric.metric_subject?.trim() ||
			m.metric_detail_fallback_name({ id: metric.id }),
		subname: metric.metric_name_en?.trim() || metric.metric_subject_en?.trim() || ''
	};
}
