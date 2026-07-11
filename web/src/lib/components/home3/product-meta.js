/**
 * @typedef {{ label: string, kind: 'text', value: string } | { label: string, kind: 'chips', items: string[] }} ProductMetaSection
 */

/**
 * @param {unknown} value
 * @returns {string}
 */
function normalizeText(value) {
	return typeof value === 'string' ? value.trim() : '';
}

/**
 * @param {unknown} value
 * @returns {string[]}
 */
function normalizeList(value) {
	return Array.isArray(value)
		? value.filter((item) => typeof item === 'string').map((item) => item.trim()).filter(Boolean)
		: [];
}

/**
 * @param {string[]} a
 * @param {string[]} b
 * @returns {boolean}
 */
function sameList(a, b) {
	return a.length === b.length && a.every((item, index) => item === b[index]);
}

/**
 * @param {Record<string, unknown> | null | undefined} product
 * @returns {ProductMetaSection[]}
 */
export function buildProductMetaSections(product) {
	const relSummary = normalizeText(product?.relation_summary);
	const relSummaryEn = normalizeText(product?.relation_summary_en);
	const reqText = normalizeText(product?.requirement_text);
	const reqTextEn = normalizeText(product?.requirement_text_en);
	const canonicalName = normalizeText(product?.canonical_name);
	const canonicalNameEn = normalizeText(product?.canonical_name_en);
	const obligationLevel = normalizeText(product?.obligation_level);
	const confidenceReason = normalizeText(product?.confidence_reason);

	/** @type {ProductMetaSection[]} */
	const sections = [];

	if (relSummary) {
		sections.push({ label: 'Relation Summary', kind: 'text', value: relSummary });
	}
	if (relSummaryEn && relSummaryEn !== relSummary) {
		sections.push({ label: 'Relation Summary (English)', kind: 'text', value: relSummaryEn });
	}
	if (reqText) {
		sections.push({ label: 'Requirement', kind: 'text', value: reqText });
	}
	if (reqTextEn && reqTextEn !== reqText) {
		sections.push({ label: 'Requirement (English)', kind: 'text', value: reqTextEn });
	}
	if (canonicalName) {
		sections.push({ label: 'Canonical Name', kind: 'text', value: canonicalName });
	}
	if (canonicalNameEn && canonicalNameEn !== canonicalName) {
		sections.push({ label: 'Canonical Name (English)', kind: 'text', value: canonicalNameEn });
	}
	if (obligationLevel) {
		sections.push({ label: 'Obligation Level', kind: 'chips', items: [obligationLevel] });
	}
	if (confidenceReason) {
		sections.push({ label: 'Confidence Reason', kind: 'text', value: confidenceReason });
	}
	return sections;
}
