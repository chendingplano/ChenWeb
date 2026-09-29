// Read-time table context for a metric (openspec change table-row-context): the
// header rows, the metric's matched rows and one neighbor data row either side,
// built by the server from kb.metrics.source_table_rows.

export type TableContextRow = {
	id: string;
	cells: string[];
	/** The row is one cell spanning every column (footnote / section row). */
	full_width?: boolean;
	header?: boolean;
	matched?: boolean;
};

export type TableContextWindow = {
	line: number;
	caption?: string;
	columns: string[];
	rows: TableContextRow[];
};

/** Longest cell text shown in the compact popup table; the full text goes in a tooltip. */
export const TABLE_CONTEXT_CELL_CHARS = 80;

export function splitTableContextRows(w: TableContextWindow): {
	head: TableContextRow[];
	body: TableContextRow[];
} {
	const rows = w.rows ?? [];
	return {
		head: rows.filter((r) => r.header),
		body: rows.filter((r) => !r.header)
	};
}

export function clipTableCell(text: string, max = TABLE_CONTEXT_CELL_CHARS): string {
	const chars = Array.from(text ?? '');
	return chars.length <= max ? text : chars.slice(0, max - 1).join('') + '…';
}
