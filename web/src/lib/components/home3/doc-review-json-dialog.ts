export type JsonDialogRow = {
	label: string;
	value: string;
	// When true, renders as a nested/indented row under the row above it (used
	// for embedding a sub-object's fields inline, e.g. related_artifact_fields).
	indent?: boolean;
	// When set, the value renders as a Line/Content table instead of `value`
	// (used for source_context: an array of {line_number, content} spans).
	sourceContext?: { lineNumber: string; content: string }[];
};

export type JsonDialogSection = {
	rows: JsonDialogRow[];
};

function isRecord(value: unknown): value is Record<string, unknown> {
	return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function displayValue(value: unknown): string {
	if (value == null) return 'null';
	if (typeof value === 'string') return value;
	if (typeof value === 'number' || typeof value === 'boolean') return String(value);
	if (Array.isArray(value) && value.every((item) => typeof item === 'string')) {
		return `[${value.map((item) => `'${item}'`).join(', ')}]`;
	}
	try {
		return JSON.stringify(value);
	} catch {
		return String(value);
	}
}

function flattenRecord(record: Record<string, unknown>): JsonDialogRow[] {
	return Object.entries(record).map(([label, value]) => ({
		label,
		value: displayValue(value)
	}));
}

function metricRecordRows(value: unknown): JsonDialogRow[] {
	if (!isRecord(value)) return [];
	const metric = isRecord(value.metric) ? value.metric : value;
	return flattenRecord(metric);
}

export function buildMatchedUnitsSections(value: unknown): JsonDialogSection[] {
	if (!Array.isArray(value)) {
		if (isRecord(value)) return [{ rows: metricRecordRows(value) }];
		return [{ rows: [{ label: 'value', value: displayValue(value) }] }];
	}

	if (!value.length) return [];
	return value
		.map((item) => ({ rows: metricRecordRows(item) }))
		.filter((section) => section.rows.length > 0);
}

// A matched_units[i] entry is shaped { metric: {...}, match_via, match_rank,
// source_doc_authority, source_record_id, source_filename, source_context }
// (review-metrics.go:465-479) — the label for a list entry is the nested
// metric's own id, falling back to a 1-based ordinal when unavailable.
export function matchedUnitLabel(value: unknown, index: number): string {
	if (isRecord(value) && isRecord(value.metric) && typeof value.metric.metric_id === 'string' && value.metric.metric_id) {
		return value.metric.metric_id;
	}
	return `Unit ${index + 1}`;
}

function isSourceContextSpans(value: unknown): value is Record<string, unknown>[] {
	return Array.isArray(value) && value.length > 0 && value.every((item) => isRecord(item) && 'content' in item);
}

function matchedUnitSiblingRow(label: string, value: unknown): JsonDialogRow {
	if (label === 'source_context' && isSourceContextSpans(value)) {
		return {
			label,
			value: `${value.length} line${value.length === 1 ? '' : 's'}`,
			sourceContext: value.map((span) => ({
				lineNumber: displayValue(span.line_number),
				content: displayValue(span.content)
			}))
		};
	}
	return { label, value: displayValue(value) };
}

// Full field list for a single matched_units[i] entry: the nested metric's
// own fields plus its match-provenance siblings (match_via, match_rank, ...),
// flattened into one row list. Unlike metricRecordRows (used by the existing
// Doc Review Logs admin screen's "Matched" button), this does NOT discard the
// sibling fields. source_context (an array of {line_number, content} spans)
// renders as a Line/Content table via JsonDialogRow.sourceContext.
export function buildMatchedUnitRows(value: unknown): JsonDialogRow[] {
	if (!isRecord(value)) return [{ label: 'value', value: displayValue(value) }];
	const { metric, ...rest } = value;
	const metricRows = isRecord(metric) ? flattenRecord(metric) : [];
	const restRows = Object.entries(rest).map(([label, v]) => matchedUnitSiblingRow(label, v));
	return [...metricRows, ...restRows];
}

// Row builder for the Finding Details panel's "View findings" dialog:
// artifact_fields is dropped (redundant with the Artifact block already shown
// in the panel), and related_artifact_fields is expanded inline as indented
// name-value rows instead of a single stringified JSON blob.
function findingRows(finding: Record<string, unknown>): JsonDialogRow[] {
	const rows: JsonDialogRow[] = [];
	for (const [key, value] of Object.entries(finding)) {
		if (key === 'artifact_fields') continue;
		if (key === 'related_artifact_fields' && isRecord(value)) {
			rows.push({ label: key, value: '' });
			for (const [subKey, subValue] of Object.entries(value)) {
				rows.push({ label: subKey, value: displayValue(subValue), indent: true });
			}
			continue;
		}
		rows.push({ label: key, value: displayValue(value) });
	}
	return rows;
}

export function buildFindingsSections(value: unknown): JsonDialogSection[] {
	if (Array.isArray(value)) {
		if (!value.length) return [];
		return value.map((item) => ({
			rows: isRecord(item) ? findingRows(item) : [{ label: 'value', value: displayValue(item) }]
		}));
	}
	if (isRecord(value)) return [{ rows: findingRows(value) }];
	return [{ rows: [{ label: 'value', value: displayValue(value) }] }];
}

export function buildJsonSections(value: unknown): JsonDialogSection[] {
	if (Array.isArray(value)) {
		if (!value.length) return [];
		return value.map((item) => ({
			rows: isRecord(item)
				? flattenRecord(item)
				: [{ label: 'value', value: displayValue(item) }]
		}));
	}

	if (isRecord(value)) return [{ rows: flattenRecord(value) }];
	return [{ rows: [{ label: 'value', value: displayValue(value) }] }];
}

export type JsonTreeNode = {
	label: string;
	value: string;
	children?: JsonTreeNode[];
};

function isPrimitive(value: unknown): boolean {
	return value == null || typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean';
}

function jsonTreeNode(label: string, value: unknown): JsonTreeNode {
	if (isPrimitive(value)) return { label, value: displayValue(value) };
	if (Array.isArray(value)) {
		if (value.every(isPrimitive)) return { label, value: displayValue(value) };
		return { label, value: `[${value.length}]`, children: value.map((item, i) => jsonTreeNode(String(i), item)) };
	}
	if (isRecord(value)) {
		const entries = Object.entries(value);
		return { label, value: `{${entries.length}}`, children: entries.map(([k, v]) => jsonTreeNode(k, v)) };
	}
	return { label, value: displayValue(value) };
}

// Builds a recursive, indentable tree of name-value nodes from arbitrary JSON,
// unlike buildJsonSections (which flattens nested objects/arrays into a single
// stringified row). Used by json-tree-dialog.svelte for the finding metadata
// dialog, where nested structure should stay legible.
export function buildJsonTree(value: unknown): JsonTreeNode[] {
	if (isRecord(value)) return Object.entries(value).map(([k, v]) => jsonTreeNode(k, v));
	if (Array.isArray(value)) return value.map((item, i) => jsonTreeNode(String(i), item));
	return [{ label: 'value', value: displayValue(value) }];
}

export function formatCompactContent(value: unknown): string {
	if (isRecord(value)) {
		const entries = Object.entries(value);
		if (entries.length === 1) return formatCompactContent(entries[0][1]);
	}

	if (Array.isArray(value)) {
		if (value.every((item) => item == null || typeof item === 'string' || typeof item === 'number' || typeof item === 'boolean')) {
			return `[${value.map((item) => item == null ? 'null' : String(item)).join(', ')}]`;
		}
	}

	return displayValue(value);
}
