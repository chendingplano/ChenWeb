/** Canonical references accepted by both reusable PDF viewers. Columns are 1-based. */
export type TableReference = {
	line: number;
	rows?: string[];
	cells?: string[];
	row_hash?: Record<string, string>;
};
export type GeometryBox = { page: number; coords: number[]; rotation?: number };
export type TableGeometry = {
	version: number;
	algorithm: string;
	coordinate_space: string;
	line_sha256?: string;
	pdf_sha256?: string;
	tables: {
		line: number;
		rows: {
			id: string;
			hash: string;
			boxes: GeometryBox[];
			cells: { id: string; boxes: GeometryBox[] }[];
		}[];
	}[];
};
export type TableHighlight = GeometryBox & { line: number; target: string };

function validBox(box: GeometryBox): boolean {
	const c = box.coords;
	return (
		Number.isInteger(box.page) &&
		box.page > 0 &&
		Array.isArray(c) &&
		c.length === 4 &&
		c.every((n) => Number.isFinite(n) && n >= 0 && n <= 1000) &&
		c[2] > c[0] &&
		c[3] > c[1]
	);
}

function unrotated(coords: number[], rotation = 0): number[] {
	const [x1, y1, x2, y2] = coords;
	switch (rotation) {
		case 90:
			return [y1, 1000 - x2, y2, 1000 - x1];
		case 180:
			return [1000 - x2, 1000 - y2, 1000 - x1, 1000 - y1];
		case 270:
			return [1000 - y2, x1, 1000 - y1, x2];
		default:
			return coords;
	}
}

/** Missing targets or stale citations stay unresolved; no table-box fallback. */
export function resolveTableReferences(
	refs: TableReference[],
	geometry: TableGeometry | null,
	respectPageRotation = true
): TableHighlight[] {
	if (
		!geometry ||
		geometry.version !== 1 ||
		geometry.algorithm !== 'canonical-table-geometry-v2' ||
		geometry.coordinate_space !== 'page-normalized-1000' ||
		!Array.isArray(geometry.tables)
	)
		return [];
	const resolved: TableHighlight[] = [];
	const seen = new Set<string>();
	const selectedRows = new Set<string>();
	for (const ref of refs) {
		const table = geometry.tables.find((t) => t.line === ref.line);
		for (const id of ref.rows ?? []) {
			const row = table?.rows?.find((r) => r.id === id);
			if (
				row &&
				(ref.row_hash?.[id] === undefined || ref.row_hash[id] === row.hash) &&
				row.boxes?.some(validBox)
			)
				selectedRows.add(`${ref.line}#${id}`);
		}
	}
	for (const ref of refs) {
		const table = geometry.tables.find((t) => t.line === ref.line);
		if (!table || !Array.isArray(table.rows)) continue;
		const targets = [
			...(ref.rows ?? []),
			...(ref.cells ?? []).filter((cell) => !selectedRows.has(`${ref.line}#${cell.split(':')[0]}`))
		];
		for (const target of targets) {
			if (!/^(?:h\d+|r[1-9]\d*)(?::c[1-9]\d*)?$/.test(target)) continue;
			const rowId = target.split(':')[0];
			const row = table.rows.find((r) => r.id === rowId);
			if (!row || (ref.row_hash?.[rowId] !== undefined && ref.row_hash[rowId] !== row.hash))
				continue;
			const boxes = target.includes(':')
				? row.cells?.find((c) => c.id === target)?.boxes
				: row.boxes;
			for (const box of boxes ?? []) {
				const c = box.coords;
				if (!validBox(box)) continue;
				const coords = respectPageRotation ? c : unrotated(c, box.rotation);
				const key = `${ref.line}#${box.page}:${coords.join(',')}`;
				if (seen.has(key)) continue;
				seen.add(key);
				resolved.push({ page: box.page, coords, line: ref.line, target });
			}
		}
	}
	return resolved.sort(
		(a, b) => a.page - b.page || a.coords[1] - b.coords[1] || a.coords[0] - b.coords[0]
	);
}
