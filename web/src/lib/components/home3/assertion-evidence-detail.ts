export type DetailRow = { key: string; value: string | null; depth: number };

export function detailRows(
	value: unknown,
	key = 'record',
	depth = 0,
	out: DetailRow[] = []
): DetailRow[] {
	if (value === null || value === undefined) {
		out.push({ key, value: 'null', depth });
		return out;
	}
	if (typeof value !== 'object') {
		out.push({ key, value: String(value), depth });
		return out;
	}
	out.push({ key, value: null, depth });
	if (Array.isArray(value)) {
		value.forEach((item, index) => detailRows(item, `[${index}]`, depth + 1, out));
	} else {
		Object.entries(value as Record<string, unknown>).forEach(([childKey, childValue]) =>
			detailRows(childValue, childKey, depth + 1, out)
		);
	}
	return out;
}
