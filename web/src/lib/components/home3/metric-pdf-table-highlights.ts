import type { TableContextWindow } from './metric-table-context';

/** PDF text coordinates normalized to the same 0–1000 page space as raw lines. */
export type PdfTextBox = { text: string; coords: number[] };
export type TableRowHighlight = { page: number; line: number; row: string; coords: number[] };
type TableMetric = {
	source_table_rows?: { line: number; rows: string[] }[] | null;
	table_context?: TableContextWindow[];
};

function normalized(text: string): string {
	return text
		.normalize('NFKC')
		.toLowerCase()
		.replace(/[^\p{L}\p{N}]/gu, '');
}

/** Locate cited rows in PDF text, including continuations absent from canonical lines.
 * Only unique cell matches are used; repeated rowspan values cannot move the box
 * into a neighboring row. An unresolved citation never falls back to the table box.
 */
export function metricTableHighlights(
	metric: TableMetric | null | undefined,
	pages: ReadonlyMap<number, PdfTextBox[]>
): TableRowHighlight[] {
	const streams = [...pages].map(([page, boxes]) => {
		let text = '';
		const ranges = boxes.map((box) => {
			const start = text.length;
			text += normalized(box.text);
			return { start, end: text.length, box };
		});
		return { page, text, ranges };
	});
	const highlights: TableRowHighlight[] = [];
	for (const ref of metric?.source_table_rows ?? []) {
		const window = metric?.table_context?.find((w) => w.line === ref.line);
		if (!window) continue;
		for (const id of ref.rows) {
			const row = window.rows.find((r) => r.id === id);
			if (!row) continue;
			const byPage = new Map<number, number[]>();
			const candidates: { page: number; boxes: PdfTextBox[]; unique: boolean }[] = [];
			for (const cell of row.cells) {
				const needle = normalized(cell);
				if (needle.length < 4) continue;
				if (window.rows.some((r) => r.id !== id && r.cells.some((c) => normalized(c) === needle)))
					continue;
				const matches = streams.flatMap((stream) => {
					const found: { stream: typeof stream; start: number }[] = [];
					for (
						let start = stream.text.indexOf(needle);
						start >= 0;
						start = stream.text.indexOf(needle, start + 1)
					) {
						found.push({ stream, start });
					}
					return found;
				});
				for (const { stream, start } of matches) {
					candidates.push({
						page: stream.page,
						unique: matches.length === 1,
						boxes: stream.ranges
							.filter((r) => r.end > start && r.start < start + needle.length && r.end > r.start)
							.map((r) => r.box)
							.filter((b) => b.coords.length === 4 && b.coords.every(Number.isFinite))
					});
				}
			}
			// First anchor the row using unique cell text. Repeated labels can then be
			// included only within the anchor's vertical extent on the same page.
			for (const unique of [true, false]) {
				for (const candidate of candidates) {
					if (candidate.unique !== unique) continue;
					const anchor = byPage.get(candidate.page);
					if (
						!unique &&
						(!anchor ||
							candidate.boxes.some((b) => b.coords[1] < anchor[1] || b.coords[3] > anchor[3]))
					)
						continue;
					for (const { coords: c } of candidate.boxes) {
						const prev = byPage.get(candidate.page);
						byPage.set(
							candidate.page,
							prev
								? [
										Math.min(prev[0], c[0]),
										Math.min(prev[1], c[1]),
										Math.max(prev[2], c[2]),
										Math.max(prev[3], c[3])
									]
								: [...c]
						);
					}
				}
			}
			for (const [page, coords] of byPage)
				highlights.push({ page, line: ref.line, row: id, coords });
		}
	}
	return highlights.sort((a, b) => a.page - b.page || a.coords[1] - b.coords[1]);
}
