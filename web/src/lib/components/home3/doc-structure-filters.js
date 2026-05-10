/**
 * @typedef {import('$lib/services/kbService').DocStructureLine} DocStructureLine
 * @typedef {'all' | 'headings' | 'paragraphs' | 'lists' | 'tables' | 'formulas'} DocStructureFilterValue
 */

export const DOC_STRUCTURE_FILTER_OPTIONS = [
	{ value: 'all', label: 'All Lines' },
	{ value: 'headings', label: 'Headings' },
	{ value: 'paragraphs', label: 'Paragraphs' },
	{ value: 'lists', label: 'Lists' },
	{ value: 'tables', label: 'Tables' },
	{ value: 'formulas', label: 'Formulas' }
];

/**
 * @param {DocStructureLine | Pick<DocStructureLine, 'line_type' | 'corrected_line_type'>} line
 * @returns {string}
 */
export function effectiveDocStructureLineType(line) {
	const corrected = line.corrected_line_type?.trim().toLowerCase() ?? '';
	if (corrected !== '' && corrected !== 'unchanged') return corrected;
	return line.line_type?.trim().toLowerCase() ?? '';
}

/**
 * @param {string} lineType
 * @returns {boolean}
 */
function isHeadingType(lineType) {
	return /^heading(?:-\d+)?$/i.test(lineType) || lineType.includes('heading');
}

/**
 * @param {string} lineType
 * @returns {boolean}
 */
function isParagraphType(lineType) {
	return lineType === 'paragraph';
}

/**
 * @param {string} lineType
 * @returns {boolean}
 */
function isListType(lineType) {
	return lineType === 'list' || lineType.includes('list-item');
}

/**
 * @param {string} lineType
 * @returns {boolean}
 */
function isTableType(lineType) {
	return lineType === 'table' || lineType.includes('table');
}

/**
 * @param {string} lineType
 * @returns {boolean}
 */
function isFormulaType(lineType) {
	return lineType.includes('formula');
}

/**
 * @param {DocStructureLine | Pick<DocStructureLine, 'line_type' | 'corrected_line_type'>} line
 * @param {DocStructureFilterValue | string} filterValue
 * @returns {boolean}
 */
export function matchesDocStructureFilter(line, filterValue) {
	const lineType = effectiveDocStructureLineType(line);
	switch (filterValue) {
		case 'headings':
			return isHeadingType(lineType);
		case 'paragraphs':
			return isParagraphType(lineType);
		case 'lists':
			return isListType(lineType);
		case 'tables':
			return isTableType(lineType);
		case 'formulas':
			return isFormulaType(lineType);
		case 'all':
		default:
			return true;
	}
}

/**
 * @param {DocStructureLine[]} lines
 * @param {DocStructureFilterValue | string} filterValue
 * @returns {DocStructureLine[]}
 */
export function filterDocStructureLines(lines, filterValue) {
	return lines.filter((line) => matchesDocStructureFilter(line, filterValue));
}

/**
 * @param {DocStructureFilterValue | string} filterValue
 * @returns {string}
 */
export function getDocStructureFilterLabel(filterValue) {
	return (
		DOC_STRUCTURE_FILTER_OPTIONS.find((option) => option.value === filterValue)?.label ??
		DOC_STRUCTURE_FILTER_OPTIONS[0].label
	);
}
