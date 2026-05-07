// @ts-nocheck

/**
 * @typedef {'text' | 'textarea' | 'datetime' | 'array' | 'json'} MetadataEditorKind
 * @typedef {{
 *   label: string;
 *   key: string;
 *   value: string;
 *   rawValue: unknown;
 *   editable: boolean;
 *   editor?: MetadataEditorKind;
 *   editKey?: string;
 *   wide?: boolean;
 *   pathLike?: boolean;
 * }} MetadataRow
 */

/**
 * @param {unknown} value
 * @returns {string}
 */
function opt(value) {
	if (value == null || value === '') return '—';
	if (typeof value === 'boolean') return value ? 'true' : 'false';
	return String(value).trim() || '—';
}

/**
 * @param {unknown} value
 * @returns {string}
 */
function formatMaybeDate(value) {
	if (typeof value !== 'string' || value.trim() === '') return '—';
	return value.replace('T', ' ').slice(0, 19);
}

/**
 * @param {unknown} value
 * @returns {string}
 */
function formatStringArray(value) {
	if (!Array.isArray(value)) return '—';
	const out = value
		.map((item) => (typeof item === 'string' ? item.trim() : String(item)))
		.filter((item) => item.length > 0);
	return out.length > 0 ? out.join(', ') : '—';
}

/**
 * @param {unknown} value
 * @returns {string[]}
 */
function parseStringArrayValue(value) {
	if (Array.isArray(value)) {
		return value
			.map((item) => (typeof item === 'string' ? item.trim() : String(item)))
			.filter((item) => item.length > 0);
	}
	return [];
}

/**
 * @param {unknown} value
 * @returns {string}
 */
function formatConfidence(value) {
	if (typeof value !== 'number' || !Number.isFinite(value)) return '—';
	return `${value} (${Math.round(value * 100)}%)`;
}

/**
 * @param {KbMetricRecord | null} metric
 * @returns {MetadataRow[]}
 */
export function buildKbMetricMetadataRows(metric) {
	if (!metric) return [];

	return [
		{ label: 'id', key: 'id', value: opt(metric.id), rawValue: metric.id, editable: false, editKey: 'field:id' },
		{ label: 'event id', key: 'event_id', value: opt(metric.event_id), rawValue: metric.event_id ?? '', editable: false, editKey: 'field:event_id' },
		{ label: 'input record id', key: 'input_record_id', value: opt(metric.input_record_id), rawValue: metric.input_record_id, editable: false, editKey: 'field:input_record_id' },
		{ label: 'metric name', key: 'metric_name', value: opt(metric.metric_name), rawValue: metric.metric_name ?? '', editable: true, editor: 'text', editKey: 'field:metric_name' },
		{ label: 'metric name en', key: 'metric_name_en', value: opt(metric.metric_name_en), rawValue: metric.metric_name_en ?? '', editable: true, editor: 'text', editKey: 'field:metric_name_en' },
		{ label: 'source line spans', key: 'source_line_spans', value: formatStringArray(metric.source_line_spans), rawValue: parseStringArrayValue(metric.source_line_spans), editable: true, editor: 'array', editKey: 'field:source_line_spans', wide: true },
		{ label: 'metric subject', key: 'metric_subject', value: opt(metric.metric_subject), rawValue: metric.metric_subject ?? '', editable: true, editor: 'text', editKey: 'field:metric_subject' },
		{ label: 'metric subject en', key: 'metric_subject_en', value: opt(metric.metric_subject_en), rawValue: metric.metric_subject_en ?? '', editable: true, editor: 'text', editKey: 'field:metric_subject_en' },
		{ label: 'metric desc', key: 'metric_desc', value: opt(metric.metric_desc), rawValue: metric.metric_desc ?? '', editable: true, editor: 'textarea', editKey: 'field:metric_desc', wide: true },
		{ label: 'metric desc en', key: 'metric_desc_en', value: opt(metric.metric_desc_en), rawValue: metric.metric_desc_en ?? '', editable: true, editor: 'textarea', editKey: 'field:metric_desc_en', wide: true },
		{ label: 'metric context', key: 'metric_context', value: opt(metric.metric_context), rawValue: metric.metric_context ?? '', editable: true, editor: 'textarea', editKey: 'field:metric_context', wide: true },
		{ label: 'metric context en', key: 'metric_context_en', value: opt(metric.metric_context_en), rawValue: metric.metric_context_en ?? '', editable: true, editor: 'textarea', editKey: 'field:metric_context_en', wide: true },
		{ label: 'metric keywords', key: 'metric_keywords', value: formatStringArray(metric.metric_keywords), rawValue: parseStringArrayValue(metric.metric_keywords), editable: true, editor: 'array', editKey: 'field:metric_keywords', wide: true },
		{ label: 'metric keywords en', key: 'metric_keywords_en', value: formatStringArray(metric.metric_keywords_en), rawValue: parseStringArrayValue(metric.metric_keywords_en), editable: true, editor: 'array', editKey: 'field:metric_keywords_en', wide: true },
		{ label: 'model name', key: 'model_name', value: opt(metric.model_name), rawValue: metric.model_name ?? '', editable: true, editor: 'text', editKey: 'field:model_name' },
		{ label: 'location type', key: 'location_type', value: opt(metric.location_type), rawValue: metric.location_type ?? '', editable: true, editor: 'text', editKey: 'field:location_type' },
		{ label: 'metric unit', key: 'metric_unit', value: opt(metric.metric_unit), rawValue: metric.metric_unit ?? '', editable: true, editor: 'text', editKey: 'field:metric_unit' },
		{ label: 'metric unit en', key: 'metric_unit_en', value: opt(metric.metric_unit_en), rawValue: metric.metric_unit_en ?? '', editable: true, editor: 'text', editKey: 'field:metric_unit_en' },
		{ label: 'metric value', key: 'metric_value', value: opt(metric.metric_value), rawValue: metric.metric_value ?? '', editable: true, editor: 'text', editKey: 'field:metric_value' },
		{ label: 'value data type', key: 'value_data_type', value: opt(metric.value_data_type), rawValue: metric.value_data_type ?? '', editable: true, editor: 'text', editKey: 'field:value_data_type' },
		{ label: 'value range type', key: 'value_range_type', value: opt(metric.value_range_type), rawValue: metric.value_range_type ?? '', editable: true, editor: 'text', editKey: 'field:value_range_type' },
		{ label: 'value class', key: 'value_class', value: opt(metric.value_class), rawValue: metric.value_class ?? '', editable: true, editor: 'text', editKey: 'field:value_class' },
		{ label: 'value class en', key: 'value_class_en', value: opt(metric.value_class_en), rawValue: metric.value_class_en ?? '', editable: true, editor: 'text', editKey: 'field:value_class_en' },
		{ label: 'definition', key: 'formula_or_definition', value: opt(metric.formula_or_definition), rawValue: metric.formula_or_definition ?? '', editable: true, editor: 'textarea', editKey: 'field:formula_or_definition', wide: true },
		{ label: 'threshold', key: 'threshold_or_target', value: opt(metric.threshold_or_target), rawValue: metric.threshold_or_target ?? '', editable: true, editor: 'text', editKey: 'field:threshold_or_target' },
		{ label: 'frequency', key: 'measurement_frequency', value: opt(metric.measurement_frequency), rawValue: metric.measurement_frequency ?? '', editable: true, editor: 'text', editKey: 'field:measurement_frequency' },
		{ label: 'confidence', key: 'confidence', value: formatConfidence(metric.confidence), rawValue: metric.confidence == null ? '' : String(metric.confidence), editable: true, editor: 'text', editKey: 'field:confidence' },
		{ label: 'is explicit', key: 'is_explicit_metric', value: opt(metric.is_explicit_metric), rawValue: metric.is_explicit_metric == null ? '' : String(metric.is_explicit_metric), editable: true, editor: 'text', editKey: 'field:is_explicit_metric' },
		{ label: 'table/section', key: 'table_name_or_section', value: opt(metric.table_name_or_section), rawValue: metric.table_name_or_section ?? '', editable: true, editor: 'text', editKey: 'field:table_name_or_section' },
		{ label: 'reasoning tags', key: 'reasoning_tags', value: formatStringArray(metric.reasoning_tags), rawValue: parseStringArrayValue(metric.reasoning_tags), editable: true, editor: 'array', editKey: 'field:reasoning_tags', wide: true },
		{ label: 'input file', key: 'input_filename', value: opt(metric.input_filename), rawValue: metric.input_filename ?? '', editable: false, editKey: 'field:input_filename', wide: true },
		{ label: 'created at', key: 'created_at', value: formatMaybeDate(metric.created_at), rawValue: metric.created_at ?? '', editable: false, editKey: 'field:created_at' }
	];
}

/**
 * @param {string} draft
 * @returns {string[]}
 */
function parseDraftArray(draft) {
	return draft
		.split(/\n|,/)
		.map((item) => item.trim())
		.filter((item) => item.length > 0);
}

/**
 * @param {string} draft
 * @returns {number | null}
 */
function parseConfidenceDraft(draft) {
	const trimmed = draft.trim();
	if (!trimmed) return null;
	const normalized = trimmed.endsWith('%') ? String(Number(trimmed.slice(0, -1).trim()) / 100) : trimmed;
	const value = Number(normalized);
	if (!Number.isFinite(value)) {
		throw new Error('Confidence must be a number like 0.82 or 82%.');
	}
	return value;
}

/**
 * @param {string} draft
 * @returns {boolean | null}
 */
function parseBooleanDraft(draft) {
	const trimmed = draft.trim().toLowerCase();
	if (!trimmed) return null;
	if (['true', '1', 'yes', 'y'].includes(trimmed)) return true;
	if (['false', '0', 'no', 'n'].includes(trimmed)) return false;
	throw new Error('Value must be true or false.');
}

/**
 * @param {KbMetricRecord} currentMetric
 * @param {MetadataRow} row
 * @param {string} draft
 * @param {MetadataEditorKind} editor
 * @returns {Record<string, unknown>}
 */
export function buildKbMetricUpdatePayloadForMetadataEdit(currentMetric, row, draft, editor) {
	if (!currentMetric) {
		throw new Error('No metric selected.');
	}
	const field = row.editKey?.startsWith('field:') ? row.editKey.slice(6) : row.key;
	if (!field) {
		throw new Error('Unknown metric field.');
	}

	if (field === 'confidence') {
		return { confidence: parseConfidenceDraft(draft) };
	}
	if (field === 'is_explicit_metric') {
		return { is_explicit_metric: parseBooleanDraft(draft) };
	}
	if (editor === 'array') {
		return { [field]: parseDraftArray(draft) };
	}

	const trimmed = draft.trim();
	return { [field]: trimmed || null };
}
