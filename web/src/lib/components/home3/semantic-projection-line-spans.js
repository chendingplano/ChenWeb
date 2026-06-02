/**
 * @typedef {{ line_number?: number; line?: number; line_no?: number; lineNo?: number; page_number?: number }} LineRef
 */

/** @param {string | number | null | undefined} value */
function toPositiveInt(value) {
	const n = typeof value === 'string' ? Number(value.trim()) : Number(value);
	if (!Number.isFinite(n)) return null;
	const i = Math.trunc(n);
	return i > 0 ? i : null;
}

/** @param {string[]} items */
function uniqueStrings(items) {
	const out = [];
	const seen = new Set();
	for (const item of items) {
		if (seen.has(item)) continue;
		seen.add(item);
		out.push(item);
	}
	return out;
}

/** @param {string | number | LineRef | null | undefined} item */
function expandLineSpan(item) {
	if (typeof item === 'string') {
		const s = item.trim();
		if (!s) return [];
		const mm = s.match(/^(\d+)\s*[:,-]\s*(\d+)$/);
		if (mm) {
			const start = parseInt(mm[1], 10);
			const end = parseInt(mm[2], 10);
			const out = [];
			for (let n = start; n <= end && n <= start + 200; n += 1) out.push(n);
			return out;
		}
		const n = toPositiveInt(s);
		return n ? [n] : [];
	}
	if (typeof item === 'number') {
		const n = toPositiveInt(item);
		return n ? [n] : [];
	}
	if (item && typeof item === 'object') {
		const n = toPositiveInt(item.line_number ?? item.line ?? item.line_no ?? item.lineNo);
		return n ? [n] : [];
	}
	return [];
}

/** @param {Array<string | number | LineRef> | null | undefined} rawSpans */
export function formatSemanticProjectionLineSpans(rawSpans) {
	if (!Array.isArray(rawSpans)) return [];
	const labels = [];
	for (const item of rawSpans) {
		if (typeof item === 'string') {
			const s = item.trim();
			if (s) labels.push(s);
			continue;
		}
		if (typeof item === 'number') {
			const n = toPositiveInt(item);
			if (n) labels.push(String(n));
			continue;
		}
		if (item && typeof item === 'object') {
			const n = toPositiveInt(item.line_number ?? item.line ?? item.line_no ?? item.lineNo);
			if (n) labels.push(String(n));
		}
	}
	return uniqueStrings(labels);
}

/**
 * @param {Array<string | number | LineRef> | null | undefined} rawSpans
 * @param {Array<LineRef> | null | undefined} rawLines
 */
export function normalizeSemanticProjectionLineRefs(rawSpans, rawLines) {
	if (!Array.isArray(rawSpans) || !Array.isArray(rawLines)) return [];
	const lineNumToPage = new Map();
	for (const line of rawLines) {
		if (!lineNumToPage.has(line.line_number)) {
			lineNumToPage.set(line.line_number, line.page_number);
		}
	}
	const refs = [];
	const seen = new Set();
	for (const item of rawSpans) {
		for (const lineNumber of expandLineSpan(item)) {
			const pageNumber = lineNumToPage.get(lineNumber);
			if (!pageNumber) continue;
			const key = `${pageNumber}:${lineNumber}`;
			if (seen.has(key)) continue;
			seen.add(key);
			refs.push({ page_number: pageNumber, line_number: lineNumber });
		}
	}
	return refs;
}
