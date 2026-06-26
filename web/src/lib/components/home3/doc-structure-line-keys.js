/**
 * @typedef {{ page_number: number, line_number: number }} DocStructureLineLike
 * @typedef {{ line: DocStructureLineLike, lineKey: string, uiKey: string }} DocStructureLineView
 */

/**
 * @param {DocStructureLineLike} line
 */
export function docStructureLineKey(line) {
	return `${line.page_number}:${line.line_number}`;
}

/**
 * Svelte keyed each blocks require unique keys. The document structure source
 * may contain duplicate page/line pairs, so duplicate occurrences get a stable
 * suffix while the first occurrence keeps the historic key.
 *
 * @template {DocStructureLineLike} T
 * @param {T[]} lines
 * @returns {{ line: T, lineKey: string, uiKey: string }[]}
 */
export function buildDocStructureLineViews(lines) {
	const seen = new Map();
	return lines.map((line) => {
		const lineKey = docStructureLineKey(line);
		const occurrence = seen.get(lineKey) ?? 0;
		seen.set(lineKey, occurrence + 1);
		return {
			line,
			lineKey,
			uiKey: occurrence === 0 ? lineKey : `${lineKey}#${occurrence + 1}`
		};
	});
}
