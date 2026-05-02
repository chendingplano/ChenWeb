/**
 * @typedef {{ id: string, label: string, categoryPath: string | null, closable: boolean }} SummaryCategoryTab
 * @typedef {{
 *   id: string,
 *   label: string,
 *   categoryPath: string,
 *   metadata: {
 *     desc: string,
 *     category_type: string,
 *     confidence: number,
 *     keywords: string[],
 *     create_time: string
 *   },
 *   childIds: string[],
 *   summaryIds: string[],
 *   expanded?: boolean
 * }} SummaryGraphNode
 */

const FIXED_GRAPH_TAB = {
	id: 'summary-graph',
	label: 'Summary Graph',
	categoryPath: null,
	closable: false
};

/**
 * @returns {SummaryCategoryTab[]}
 */
export function createSummaryGraphTabs() {
	return [{ ...FIXED_GRAPH_TAB }];
}

/**
 * @param {SummaryCategoryTab[]} tabs
 * @param {string} categoryPath
 * @returns {{ tabs: SummaryCategoryTab[], activeTabId: string }}
 */
export function openCategorySummaryTab(tabs, categoryPath) {
	const nextId = `category:${categoryPath}`;
	const existing = tabs.find((tab) => tab.id === nextId);
	if (existing) {
		return { tabs, activeTabId: existing.id };
	}

	return {
		tabs: [
			...tabs,
			{
				id: nextId,
				label: categoryPath,
				categoryPath,
				closable: true
			}
		],
		activeTabId: nextId
	};
}

/**
 * @param {SummaryGraphNode[]} nodes
 * @param {string} nodeId
 * @returns {SummaryGraphNode[]}
 */
export function toggleNodeExpanded(nodes, nodeId) {
	return nodes.map((node) =>
		node.id === nodeId ? { ...node, expanded: !node.expanded } : node
	);
}

/**
 * @param {SummaryGraphNode[]} nodes
 * @param {string} nodeId
 * @param {string} label
 * @returns {SummaryGraphNode[]}
 */
export function renameNode(nodes, nodeId, label) {
	return nodes.map((node) => (node.id === nodeId ? { ...node, label } : node));
}

/**
 * @param {SummaryGraphNode[]} nodes
 * @param {string} parentId
 * @param {SummaryGraphNode} childNode
 * @returns {SummaryGraphNode[]}
 */
export function addChildNode(nodes, parentId, childNode) {
	return [
		...nodes.map((node) =>
			node.id === parentId ? { ...node, childIds: [...node.childIds, childNode.id] } : node
		),
		childNode
	];
}

/**
 * @param {SummaryGraphNode[]} nodes
 * @param {string} nodeId
 * @returns {SummaryGraphNode[]}
 */
export function deleteNode(nodes, nodeId) {
	return nodes
		.filter((node) => node.id !== nodeId)
		.map((node) => ({
			...node,
			childIds: node.childIds.filter((childId) => childId !== nodeId)
		}));
}

/**
 * @param {SummaryGraphNode[]} nodes
 * @param {string} sourceId
 * @param {string} targetId
 * @returns {SummaryGraphNode[]}
 */
export function mergeNodes(nodes, sourceId, targetId) {
	const source = nodes.find((node) => node.id === sourceId);
	if (!source || sourceId === targetId) return nodes;

	return nodes
		.filter((node) => node.id !== sourceId)
		.map((node) => {
			if (node.id === targetId) {
				return {
					...node,
					childIds: [...new Set([...node.childIds, ...source.childIds])],
					summaryIds: [...new Set([...(node.summaryIds ?? []), ...(source.summaryIds ?? [])])]
				};
			}

			return {
				...node,
				childIds: node.childIds.filter((childId) => childId !== sourceId)
			};
		});
}

/**
 * @param {SummaryGraphNode[]} nodes
 * @param {string} nodeId
 * @param {string[]} labels
 * @returns {SummaryGraphNode[]}
 */
export function splitNode(nodes, nodeId, labels) {
	const target = nodes.find((node) => node.id === nodeId);
	if (!target) return nodes;

	const cleanLabels = labels.map((label) => label.trim()).filter(Boolean);
	if (cleanLabels.length < 2) return nodes;

	const newNodes = cleanLabels.map((label, index) => ({
		id: `${nodeId}/split-${index + 1}`,
		categoryPath: `${target.categoryPath}/${label.toLowerCase().replace(/\s+/g, '-')}`,
		label,
		metadata: {
			desc: `Split from ${target.label}`,
			category_type: target.metadata.category_type,
			confidence: target.metadata.confidence,
			keywords: [...target.metadata.keywords],
			create_time: target.metadata.create_time
		},
		childIds: [],
		summaryIds: [],
		expanded: false
	}));

	return [
		...nodes.map((node) =>
			node.id === nodeId
				? {
						...node,
						childIds: [...node.childIds, ...newNodes.map((child) => child.id)],
						expanded: true
					}
				: node
		),
		...newNodes
	];
}
