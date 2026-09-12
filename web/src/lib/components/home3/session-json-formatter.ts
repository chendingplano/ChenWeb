export function formatSessionJson(value: unknown): string {
	return renderValue(parseSerializedJson(value), 0);
}

function parseSerializedJson(value: unknown): unknown {
	if (typeof value !== 'string') return value;

	try {
		return JSON.parse(value);
	} catch {
		return value;
	}
}

function escapeHtml(value: string): string {
	return value
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;')
		.replace(/'/g, '&#39;');
}

function renderValue(value: unknown, depth: number): string {
	if (value === null) return '<span class="session-json-value session-json-null">null</span>';
	if (typeof value === 'string') {
		return `<span class="session-json-value session-json-string">${escapeHtml(value)}</span>`;
	}
	if (typeof value === 'boolean' || typeof value === 'number' || typeof value === 'bigint') {
		return `<span class="session-json-value session-json-primitive">${escapeHtml(String(value))}</span>`;
	}
	if (typeof value === 'undefined') {
		return '<span class="session-json-value session-json-null">—</span>';
	}

	if (Array.isArray(value)) {
		if (value.length === 0) return '<span class="session-json-value session-json-null">[]</span>';
		return `<div class="session-json-group session-json-array" style="--session-json-depth:${depth};">${value
			.map((item, index) => renderEntry(`[${index}]`, item, depth))
			.join('')}</div>`;
	}

	const entries = Object.entries(value as Record<string, unknown>);
	if (entries.length === 0) return '<span class="session-json-value session-json-null">—</span>';
	return `<div class="session-json-group session-json-object" style="--session-json-depth:${depth};">${entries
		.map(([key, child]) => renderEntry(key, child, depth))
		.join('')}</div>`;
}

function renderEntry(key: string, value: unknown, depth: number): string {
	const isComplex = value !== null && typeof value === 'object';
	if (isComplex) {
		return `<div class="session-json-nested" style="--session-json-depth:${depth};"><div class="session-json-key session-json-group-key">${escapeHtml(key)}</div>${renderValue(value, depth + 1)}</div>`;
	}
	return `<div class="session-json-row" style="--session-json-depth:${depth};"><span class="session-json-key">${escapeHtml(key)}</span>${renderValue(value, depth)}</div>`;
}
