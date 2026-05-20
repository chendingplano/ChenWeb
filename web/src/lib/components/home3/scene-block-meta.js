function normalizeText(value) {
	return typeof value === 'string' ? value.trim() : '';
}

function normalizeList(value) {
	return Array.isArray(value)
		? value.filter((item) => typeof item === 'string').map((item) => item.trim()).filter(Boolean)
		: [];
}

function sameList(a, b) {
	return a.length === b.length && a.every((item, index) => item === b[index]);
}

export function buildSceneBlockMetaSections(block) {
	const summary = normalizeText(block?.summary);
	const summaryEn = normalizeText(block?.summary_en);
	const keywords = normalizeList(block?.keywords);
	const keywordsEn = normalizeList(block?.keywords_en);
	const states = normalizeList(block?.states);
	const statesEn = normalizeList(block?.states_en);

	const sections = [];
	if (summary) {
		sections.push({ label: 'Summary', kind: 'text', value: summary });
	}
	if (summaryEn && summaryEn !== summary) {
		sections.push({ label: 'Summary (English)', kind: 'text', value: summaryEn });
	}
	if (keywords.length) {
		sections.push({ label: 'Keywords', kind: 'chips', items: keywords });
	}
	if (keywordsEn.length && !sameList(keywordsEn, keywords)) {
		sections.push({ label: 'Keywords (English)', kind: 'chips', items: keywordsEn });
	}
	if (states.length) {
		sections.push({ label: 'STATES', kind: 'lines', items: states });
	}
	if (statesEn.length && !sameList(statesEn, states)) {
		sections.push({ label: 'STATES (ENGLISH)', kind: 'lines', items: statesEn });
	}
	return sections;
}
