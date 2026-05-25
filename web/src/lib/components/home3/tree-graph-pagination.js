/**
 * @param {string} parentId
 * @param {number} pageNum
 * @returns {string}
 */
export function pageVirtualId(parentId, pageNum) {
	return `|pg|${parentId}|${pageNum}`;
}

/**
 * @param {string} parentId
 * @param {number} pageNum
 * @returns {string}
 */
export function nextVirtualId(parentId, pageNum) {
	return `|nx|${parentId}|${pageNum}`;
}

/**
 * @param {string} id
 * @returns {boolean}
 */
export function isVirtualPaginationId(id) {
	return id.startsWith('|pg|') || id.startsWith('|nx|');
}

/**
 * @param {string} id
 * @returns {{ parentId: string, nextPage: number } | null}
 */
export function parseNextVirtualId(id) {
	if (!id.startsWith('|nx|')) return null;
	const body = id.slice(4);
	const lastPipe = body.lastIndexOf('|');
	if (lastPipe === -1) return null;
	const parentId = body.slice(0, lastPipe);
	const nextPage = parseInt(body.slice(lastPipe + 1), 10);
	return Number.isFinite(nextPage) && nextPage > 0 ? { parentId, nextPage } : null;
}

/**
 * @template TChild
 * @template TVirtualNode
 * @param {{
 *   parentId: string,
 *   allChildren: TChild[],
 *   pageSize: number,
 *   revealedPages: number,
 *   buildChildNode: (child: TChild) => TVirtualNode,
 *   buildPageNode: (args: { parentId: string, pageNum: number, totalPages: number, pageChildren: TVirtualNode[] }) => TVirtualNode,
 *   buildNextNode: (args: { parentId: string, pageNum: number }) => TVirtualNode
 * }} params
 * @returns {TVirtualNode[]}
 */
export function buildPaginatedChildren({
	parentId,
	allChildren,
	pageSize,
	revealedPages,
	buildChildNode,
	buildPageNode,
	buildNextNode
}) {
	const total = allChildren.length;
	const totalPages = Math.ceil(total / pageSize);
	if (totalPages <= 1) return allChildren.map(buildChildNode);

	const visiblePageCount = Math.max(1, Math.min(revealedPages, totalPages));
	const pageLevelNodes = [];

	for (let pageNum = 1; pageNum <= visiblePageCount; pageNum += 1) {
		const start = (pageNum - 1) * pageSize;
		const end = Math.min(start + pageSize, total);
		pageLevelNodes.push(
			buildPageNode({
				parentId,
				pageNum,
				totalPages,
				pageChildren: allChildren.slice(start, end).map(buildChildNode)
			})
		);
	}

	if (visiblePageCount < totalPages) {
		pageLevelNodes.push(
			buildNextNode({
				parentId,
				pageNum: visiblePageCount + 1
			})
		);
	}

	return pageLevelNodes;
}
