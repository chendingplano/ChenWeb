/**
 * @typedef {{ id: string, childIds: string[], expanded?: boolean }} GraphNode
 */

/**
 * @param {GraphNode[]} nodes
 * @returns {number}
 */
export function getVisibleTreeDepth(nodes) {
	const byId = new Map(nodes.map((node) => [node.id, node]));
	const childIds = new Set(nodes.flatMap((node) => node.childIds ?? []));
	const roots = nodes.filter((node) => !childIds.has(node.id));
	let maxDepth = 0;

	/**
	 * @param {GraphNode} node
	 * @param {number} depth
	 */
	function visit(node, depth) {
		maxDepth = Math.max(maxDepth, depth);
		if (!node.expanded) return;
		for (const childId of node.childIds ?? []) {
			const child = byId.get(childId);
			if (child) visit(child, depth + 1);
		}
	}

	for (const root of roots) visit(root, 0);
	return maxDepth;
}

/**
 * ECharts tree LR spacing is `series.width / layoutDepth`. The graph views use
 * one hidden root node, so the ECharts layout depth is one level deeper than the
 * visible category tree.
 *
 * @param {{ visibleDepth: number, parentChildDistance: number, hiddenRootDepth?: number, minWidth?: number }} params
 * @returns {number}
 */
export function getFixedTreeLayoutWidth({
	visibleDepth,
	parentChildDistance,
	hiddenRootDepth = 1,
	minWidth = parentChildDistance
}) {
	const depth = Math.max(1, Math.floor(Number(visibleDepth) || 0) + hiddenRootDepth);
	const requestedWidth = depth * Math.max(1, Number(parentChildDistance) || 1);
	return Math.max(minWidth, requestedWidth);
}

/**
 * @param {{ visiblePageCount: number, pageHeight: number, minHeight?: number }} params
 * @returns {number}
 */
export function getFixedTreeLayoutHeight({
	visiblePageCount,
	pageHeight,
	minHeight = pageHeight
}) {
	const pageCount = Math.max(1, Math.floor(Number(visiblePageCount) || 0));
	const nextPageHeight = Math.max(1, Number(pageHeight) || 1);
	return Math.max(minHeight, pageCount * nextPageHeight);
}

/**
 * @param {{
 *   rect: { left: number, right: number, top: number, bottom: number },
 *   offsetX: number,
 *   offsetY: number,
 *   stageWidth: number,
 *   stageHeight: number,
 *   margin?: number
 * }} params
 * @returns {{ x: number, y: number, changed: boolean }}
 */
export function getPanOffsetToRevealRect({
	rect,
	offsetX,
	offsetY,
	stageWidth,
	stageHeight,
	margin = 72
}) {
	let nextX = offsetX;
	let nextY = offsetY;
	const rightLimit = Math.max(margin, stageWidth - margin);
	const bottomLimit = Math.max(margin, stageHeight - margin);

	if (rect.right > rightLimit) {
		nextX -= rect.right - rightLimit;
	} else if (rect.left < margin) {
		nextX += margin - rect.left;
	}

	if (rect.bottom > bottomLimit) {
		nextY -= rect.bottom - bottomLimit;
	} else if (rect.top < margin) {
		nextY += margin - rect.top;
	}

	return {
		x: nextX,
		y: nextY,
		changed: nextX !== offsetX || nextY !== offsetY
	};
}
