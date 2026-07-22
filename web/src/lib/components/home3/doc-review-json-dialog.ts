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

// Like displayValue, but yields '' (not the literal string 'null') for an
// absent value, so optional fields such as a related artifact's relationship
// or summary can be conditionally hidden instead of rendering "null".
function optionalDisplayValue(value: unknown): string {
	return value == null ? '' : displayValue(value);
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

// Sibling fields on a matched_units[i] entry that are NOT the nested artifact
// object (review-metrics.go/review-provisions.go/review-inventory-items.go/
// review-entities.go each key the artifact differently: "metric", "provision",
// "item", "entity"). Used to find the artifact generically without hardcoding
// its key per reviewer type.
const MATCHED_UNIT_SIBLING_KEYS = new Set([
	'source_record_id',
	'source_filename',
	'source_doc_authority',
	'match_via',
	'match_rank',
	'confidence',
	'source_context'
]);

// Parses one line-span string ("14", "14-16", or "14:16") into an inclusive
// range of line numbers. Mirrors parseArtifactSpan (review-artifact-window.go:62).
function expandLineSpan(span: string): number[] {
	const s = span.trim();
	const sepIndex = (() => {
		const dash = s.indexOf('-');
		const colon = s.indexOf(':');
		if (dash > 0 && (colon < 0 || dash < colon)) return dash;
		return colon;
	})();
	if (sepIndex > 0) {
		const a = parseInt(s.slice(0, sepIndex).trim(), 10);
		const b = parseInt(s.slice(sepIndex + 1).trim(), 10);
		if (Number.isInteger(a) && Number.isInteger(b) && a > 0 && b >= a) {
			return Array.from({ length: b - a + 1 }, (_, i) => a + i);
		}
		return [];
	}
	const n = parseInt(s, 10);
	return Number.isInteger(n) && n > 0 ? [n] : [];
}

// Expands an array of line-span strings ("14", "14-16", "14:16") into a
// sorted, de-duplicated list of line numbers. Used to resolve the PDF
// panel's target lines from an artifact's source_line_spans field, e.g.
// after fetching a related_artifacts[i] entry's target artifact.
export function lineNumbersFromSpans(spans: unknown): number[] {
	if (!Array.isArray(spans)) return [];
	const out = new Set<number>();
	for (const span of spans) {
		if (typeof span !== 'string') continue;
		for (const n of expandLineSpan(span)) out.add(n);
	}
	return [...out].sort((a, b) => a - b);
}

// Resolves the {recordId, lineNumbers} target for jumping the PDF panel to a
// matched_units[i] entry's source document. Returns null when the entry
// carries no usable source_record_id. lineNumbers prefers the nested
// artifact's own source_line_spans, falling back to source_context's
// line_number values, and may be empty (still enables switching documents).
export function matchedUnitFocusTarget(unit: unknown): { recordId: number; lineNumbers: number[] } | null {
	if (!isRecord(unit)) return null;
	const recordId = Number(unit.source_record_id);
	if (!Number.isFinite(recordId) || recordId <= 0) return null;

	const artifact = Object.entries(unit).find(
		([key, v]) => !MATCHED_UNIT_SIBLING_KEYS.has(key) && isRecord(v)
	)?.[1] as Record<string, unknown> | undefined;

	const spans = artifact && Array.isArray(artifact.source_line_spans) ? artifact.source_line_spans : [];
	const fromSpans = lineNumbersFromSpans(spans);
	if (fromSpans.length > 0) {
		return { recordId, lineNumbers: fromSpans };
	}

	const context = isSourceContextSpans(unit.source_context) ? unit.source_context : [];
	const fromContext = new Set<number>();
	for (const span of context) {
		const n = Number(span.line_number);
		if (Number.isInteger(n) && n > 0) fromContext.add(n);
	}
	return { recordId, lineNumbers: [...fromContext].sort((a, b) => a - b) };
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

// Parses a value that may be a JSON-encoded string (as opposed to an already-
// parsed object/array) so fields like metadata.en, which come back from the
// API as stringified JSON, can be expanded instead of shown as a raw blob.
function parseJsonLike(value: unknown): unknown {
	if (typeof value !== 'string') return value;
	const trimmed = value.trim();
	if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) return value;
	try {
		return JSON.parse(trimmed);
	} catch {
		return value;
	}
}

// Expands a single metadata field into name-value rows: objects become an
// indented row per key, arrays of objects become indented rows per key
// prefixed with the item's index, and anything else falls back to a single
// row (same as flattenRecord). Used for the Metadata dialog, where fields
// such as en/artifact_fields/related_artifacts arrive as JSON-encoded
// strings that would otherwise render as an unreadable blob.
function metadataFieldRows(key: string, rawValue: unknown): JsonDialogRow[] {
	const value = parseJsonLike(rawValue);

	if (isRecord(value)) {
		return [
			{ label: key, value: '' },
			...Object.entries(value).map(([subKey, subValue]) => ({
				label: subKey,
				value: displayValue(subValue),
				indent: true
			}))
		];
	}

	if (Array.isArray(value) && value.length && value.every(isRecord)) {
		return [
			{ label: key, value: `${value.length} item${value.length === 1 ? '' : 's'}` },
			...value.flatMap((item, index) =>
				Object.entries(item as Record<string, unknown>).map(([subKey, subValue]) => ({
					label: `[${index}] ${subKey}`,
					value: displayValue(subValue),
					indent: true
				}))
			)
		];
	}

	return [{ label: key, value: displayValue(rawValue) }];
}

// Like buildJsonSections, but expands any field whose value is a JSON-encoded
// object/array (e.g. metadata.en, metadata.artifact_fields,
// metadata.related_artifacts) into indented name-value rows instead of a raw
// stringified blob. Used by the Metadata dialog.
export function buildMetadataSections(value: unknown): JsonDialogSection[] {
	if (!isRecord(value)) return buildJsonSections(value);
	return [{ rows: Object.entries(value).flatMap(([key, v]) => metadataFieldRows(key, v)) }];
}

export type RelatedArtifact = {
	relationship: string;
	related_record_id: string;
	related_artifact_id: string;
	summary: string;
	// The related artifact's own raw field values (e.g. metric_name,
	// metric_value), present only for the singular metadata shape below,
	// where there is no LLM-authored summary to show instead.
	fields: JsonDialogRow[];
};

// Pulls a finding's related-artifact reference out of its metadata, which the
// doc-review pipeline emits in one of two shapes (models.go
// FindingMetadataEnvelope):
//  - plural `related_artifacts`: an array of {summary, relationship,
//    related_record_id, related_artifact_id}, used only by the metrics
//    reviewer's multi-candidate "analysis" finding_type row (one finding per
//    metric, one array entry per candidate compared against it).
//  - singular `related_artifact_id`/`related_record_id`/
//    `analysis_relationship`/`related_artifact_fields`, used by every other
//    finding shape (e.g. ordinary "issue" findings): exactly one related
//    artifact, carrying its own raw field values instead of a summary.
// Either shape's values may arrive JSON-encoded as a string, like other
// metadata fields — see parseJsonLike. Normalized into one list so the
// Finding Details panel can show it inline, without requiring the "View
// metadata" dialog.
export function relatedArtifactsFromMetadata(metadata: unknown): RelatedArtifact[] {
	if (!isRecord(metadata)) return [];

	const plural = parseJsonLike(metadata.related_artifacts);
	if (Array.isArray(plural) && plural.length > 0) {
		return plural.filter(isRecord).map((item) => ({
			relationship: optionalDisplayValue(item.relationship),
			related_record_id: displayValue(item.related_record_id),
			related_artifact_id: displayValue(item.related_artifact_id),
			summary: optionalDisplayValue(item.summary),
			fields: []
		}));
	}

	if (metadata.related_artifact_id === undefined && metadata.related_record_id === undefined) {
		return [];
	}
	const relatedFields = parseJsonLike(metadata.related_artifact_fields);
	return [
		{
			relationship: optionalDisplayValue(metadata.analysis_relationship),
			related_record_id: displayValue(metadata.related_record_id),
			related_artifact_id: displayValue(metadata.related_artifact_id),
			summary: '',
			fields: isRecord(relatedFields) ? flattenRecord(relatedFields) : []
		}
	];
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
