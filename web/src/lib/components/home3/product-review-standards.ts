// Product Review's report groups by the source document number, never its title.
export const standardCategories = [
	'national', 'international', 'eu', 'us', 'industry', 'local', 'association', 'enterprise', 'other'
] as const;
export type StandardCategory = typeof standardCategories[number];

// Restore saved layouts defensively and append categories introduced in later versions.
export function normalizeStandardOrder(saved: unknown): StandardCategory[] {
	const valid = Array.isArray(saved) ? saved.filter((value): value is StandardCategory =>
		standardCategories.includes(value as StandardCategory)) : [];
	return [...new Set([...valid, ...standardCategories])];
}

export function moveStandardCategory(order: StandardCategory[], category: StandardCategory, target: StandardCategory) {
	const next = [...order];
	const from = next.indexOf(category);
	const to = next.indexOf(target);
	if (from < 0 || to < 0 || from === to) return next;
	next.splice(from, 1);
	next.splice(to, 0, category);
	return next;
}

export function classifyStandard(docNo = ''): StandardCategory {
	const value = docNo.toUpperCase().replace(/／/g, '/').replace(/\s+/g, '');
	if (/^GB(?:\/[TZ])?\d/.test(value)) return 'national';
	if (/^ISO(?=[/\d:-]|$)/.test(value)) return 'international';
	if (/^EN(?=\d|ISO|IEC|$)/.test(value)) return 'eu';
	if (/^ASTM(?=[A-Z]?\d|$)/.test(value)) return 'us';
	if (/^DB\d{2,6}(?=\/|\d|$)/.test(value)) return 'local';
	if (/^T\/[A-Z0-9]/.test(value)) return 'association';
	if (/^Q\/[A-Z0-9]/.test(value)) return 'enterprise';
	if (/^[A-Z]{1,6}\/T\d/.test(value)) return 'industry';
	return 'other';
}

export function groupStandardMetrics<T extends { artifact_type: string; doc_no?: string }>(rows: T[]) {
	const groups = standardCategories.map(category => ({ category, rows: [] as T[] }));
	for (const row of rows) {
		if (row.artifact_type === 'metric') {
			groups.find(group => group.category === classifyStandard(row.doc_no))!.rows.push(row);
		}
	}
	return groups;
}
