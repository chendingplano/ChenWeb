/**
 * @typedef {{ id: string, label: string, categoryPath: string | null, closable: boolean }} TopicCategoryTab
 * @typedef {{
 *   id: string,
 *   label: string,
 *   categoryPath: string,
 *   metadata: {
 *     desc: string,
 *     confidence: number,
 *     keywords: string[],
 *     create_time: string
 *   },
 *   childIds: string[],
 *   topicIds: string[],
 *   hasTopicsFile: boolean,
 *   expanded?: boolean
 * }} TopicGraphNode
 */

const FIXED_GRAPH_TAB = {
	id: 'topic-graph',
	label: 'Semantic Web',
	categoryPath: null,
	closable: false
};

/**
 * @returns {TopicCategoryTab[]}
 */
export function createTopicGraphTabs() {
	return [{ ...FIXED_GRAPH_TAB }];
}

/**
 * @param {TopicCategoryTab[]} tabs
 * @param {string} categoryPath
 * @returns {{ tabs: TopicCategoryTab[], activeTabId: string }}
 */
export function openCategoryTopicTab(tabs, categoryPath) {
	const nextId = `topic-category:${categoryPath}`;
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
 * @param {TopicGraphNode[]} nodes
 * @param {string} nodeId
 * @returns {TopicGraphNode[]}
 */
export function toggleNodeExpanded(nodes, nodeId) {
	return nodes.map((node) =>
		node.id === nodeId ? { ...node, expanded: !node.expanded } : node
	);
}

/**
 * @param {TopicGraphNode[]} nodes
 * @param {string} nodeId
 * @param {string} label
 * @returns {TopicGraphNode[]}
 */
export function renameNode(nodes, nodeId, label) {
	return nodes.map((node) => (node.id === nodeId ? { ...node, label } : node));
}

/**
 * @param {TopicGraphNode[]} nodes
 * @param {string} parentId
 * @param {TopicGraphNode} childNode
 * @returns {TopicGraphNode[]}
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
 * @param {TopicGraphNode[]} nodes
 * @param {string} nodeId
 * @returns {TopicGraphNode[]}
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
 * @param {TopicGraphNode[]} nodes
 * @param {string} sourceId
 * @param {string} targetId
 * @returns {TopicGraphNode[]}
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
					topicIds: [...new Set([...(node.topicIds ?? []), ...(source.topicIds ?? [])])]
				};
			}
			return {
				...node,
				childIds: node.childIds.filter((childId) => childId !== sourceId)
			};
		});
}

/**
 * @param {TopicGraphNode[]} nodes
 * @param {string} nodeId
 * @param {string[]} labels
 * @returns {TopicGraphNode[]}
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
			confidence: target.metadata.confidence,
			keywords: [...target.metadata.keywords],
			create_time: target.metadata.create_time
		},
		childIds: [],
		topicIds: [],
		hasTopicsFile: false,
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
