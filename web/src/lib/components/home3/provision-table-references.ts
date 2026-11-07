import type { TableReference } from './pdf-table-geometry';

/** Older gold runs cite rows even for cell provisions. Narrow only a unique text match. */
export function provisionTableReferences(
	locationType: string | undefined,
	provision: string | undefined,
	references: TableReference[],
	lines: readonly { line_number: number; content: string }[]
): TableReference[] {
	if (locationType !== 'table_cell' || !provision?.trim() || typeof DOMParser === 'undefined')
		return references;
	const normalize = (text: string) => text.normalize('NFKC').replace(/\s+/g, '');
	const quote = normalize(provision);
	return references.map((ref) => {
		if (ref.cells?.length || ref.rows?.length !== 1) return ref;
		const content = lines.find((line) => line.line_number === ref.line)?.content;
		if (!content) return ref;
		const table = new DOMParser().parseFromString(content, 'text/html').querySelector('table');
		if (!table) return ref;
		const carried = new Map<number, number>();
		const matches: number[] = [];
		for (const row of table.querySelectorAll('tr')) {
			let column = 1;
			for (const cell of row.children) {
				if (!['TD', 'TH'].includes(cell.tagName)) continue;
				while ((carried.get(column) ?? 0) > 0) column++;
				const colspan = Math.max(1, Number(cell.getAttribute('colspan')) || 1);
				const rowspan = Math.max(1, Number(cell.getAttribute('rowspan')) || 1);
				if (normalize(cell.textContent ?? '').includes(quote)) matches.push(column);
				for (let offset = 0; offset < colspan; offset++) {
					carried.set(column + offset, rowspan);
				}
				column += colspan;
			}
			for (const [col, remaining] of carried) carried.set(col, remaining - 1);
		}
		return matches.length === 1
			? { ...ref, cells: [`${ref.rows[0]}:c${matches[0]}`] }
			: ref;
	});
}
