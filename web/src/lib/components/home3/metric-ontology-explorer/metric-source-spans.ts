// Turn a kb.metrics row's `source_line_spans` into concrete PDF locations for the
// source pane. Spans are line-number tokens only (90, "90", "98:99", {line_number: 90});
// page numbers and box coordinates come from the record's raw lines
// (GET /api/v1/kb/raw-lines). This mirrors metric-mgmt-view's normalizeMetricSpans.

import type { RawLine } from '$lib/services/kbService';

export type PageLine = { page_number: number; line_number: number };

const RANGE_CAP = 200;

/** Flatten source_line_spans into a de-duped, ascending list of line numbers. */
export function lineNumbersFromSpans(raw: unknown): number[] {
	if (!Array.isArray(raw)) return [];
	const nums = new Set<number>();
	const add = (n: number) => {
		if (Number.isFinite(n) && n > 0) nums.add(Math.trunc(n));
	};
	for (const item of raw) {
		if (typeof item === 'number') {
			add(item);
		} else if (typeof item === 'string') {
			const s = item.trim();
			const range = s.match(/^(\d+)\s*[:,-]\s*(\d+)$/);
			if (range) {
				const a = parseInt(range[1], 10);
				const b = parseInt(range[2], 10);
				for (let n = a; n <= b && n <= a + RANGE_CAP; n++) add(n);
			} else {
				add(parseInt(s, 10));
			}
		} else if (item && typeof item === 'object') {
			const o = item as Record<string, unknown>;
			const v = o.line_number ?? o.line ?? o.line_no ?? o.lineNo;
			add(typeof v === 'number' ? v : parseInt(String(v ?? ''), 10));
		}
	}
	return [...nums].sort((a, b) => a - b);
}

/** Resolve a metric's line spans to {page, line}, using the record's raw lines
 *  to look up each line's page. Lines with no matching raw line are dropped. */
export function resolveMetricSpans(rawSpans: unknown, rawLines: readonly RawLine[]): PageLine[] {
	const lineToPage = new Map<number, number>();
	for (const ln of rawLines) {
		if (!lineToPage.has(ln.line_number)) lineToPage.set(ln.line_number, ln.page_number);
	}
	const out: PageLine[] = [];
	for (const line_number of lineNumbersFromSpans(rawSpans)) {
		const page_number = lineToPage.get(line_number);
		if (page_number && page_number > 0) out.push({ page_number, line_number });
	}
	return out;
}

/** The record id encoded in a canonical metric_id — "<record_id>_mtc_<seqno>"
 *  (a legacy "<record_id>_<seqno>" form also exists). */
export function recordIdFromMetricId(metricId: string): number | null {
	const m = (metricId ?? '').trim().match(/^(\d+)_(?:mtc_)?\d+$/);
	if (!m) return null;
	const n = parseInt(m[1], 10);
	return Number.isFinite(n) && n > 0 ? n : null;
}
