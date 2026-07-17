export type JsonDialogRow = {
	label: string;
	value: string;
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
