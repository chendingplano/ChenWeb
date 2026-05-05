/**
 * @typedef {{
 *   id: string,
 *   label: string,
 *   categoryPath: string,
 *   metadata: {
 *     desc?: string,
 *     keywords?: string[],
 *     create_time?: string
 *   },
 *   childIds?: string[]
 * }} GraphFilterNode
 *
 * @typedef {{
 *   keywords?: string,
 *   startTime?: string,
 *   endTime?: string
 * }} LevelFilterCriteria
 */

/**
 * @param {string} categoryPath
 * @returns {number}
 */
export function getCategoryPathLevel(categoryPath) {
	const clean = String(categoryPath ?? '').trim();
	if (!clean) return 0;
	return clean.split('/').filter(Boolean).length - 1;
}

/**
 * @param {GraphFilterNode[]} nodes
 * @param {string | null | undefined} selectedNodeId
 * @returns {GraphFilterNode[]}
 */
export function getNodesInSelectedLevel(nodes, selectedNodeId) {
	const selected = nodes.find((node) => node.id === selectedNodeId);
	if (!selected) return [];
	const selectedLevel = getCategoryPathLevel(selected.categoryPath);
	return nodes.filter((node) => getCategoryPathLevel(node.categoryPath) === selectedLevel);
}

/**
 * @param {GraphFilterNode[]} nodes
 * @param {string | null | undefined} selectedNodeId
 * @param {LevelFilterCriteria} criteria
 * @returns {string[]}
 */
export function filterNodesInSelectedLevel(nodes, selectedNodeId, criteria) {
	const keywords = String(criteria.keywords ?? '')
		.split(/[\s,]+/)
		.map((keyword) => keyword.trim().toLowerCase())
		.filter(Boolean);
	const startTime = String(criteria.startTime ?? '').trim();
	const endTime = String(criteria.endTime ?? '').trim();

	return getNodesInSelectedLevel(nodes, selectedNodeId)
		.filter((node) => {
			const haystack = [
				node.label,
				node.categoryPath,
				node.metadata?.desc,
				...(node.metadata?.keywords ?? [])
			]
				.join(' ')
				.toLowerCase();
			if (keywords.length > 0 && !keywords.every((keyword) => haystack.includes(keyword))) {
				return false;
			}
			const createTime = String(node.metadata?.create_time ?? '').trim();
			if (startTime && createTime < startTime) return false;
			if (endTime && createTime > endTime) return false;
			return true;
		})
		.map((node) => node.id);
}
