export type ViewMode = 'graph' | 'left-right' | 'top-bottom' | 'bottom-top' | 'right-left';
type RecordNode = { id: number; name: string; value?: string };
export type HierarchyNode = { id: string; name: string; value?: string; children?: HierarchyNode[] };

export function buildDocumentHierarchy(
	document: RecordNode,
	metrics: RecordNode[],
	references: RecordNode[],
	labels: { metrics: string; references: string }
) {
	const rootId = `input:${document.id}`;
	return {
		id: rootId,
		name: document.name,
		children: (['metrics', 'references'] as const).map((kind) => ({
			id: `${rootId}:${kind}`,
			name: labels[kind],
			children: (kind === 'metrics' ? metrics : references).map((record) => ({
				...record,
				id: `${rootId}:${kind}:${record.id}`
			}))
		}))
	};
}

export function flattenHierarchy(root: HierarchyNode) {
	const nodes: { id: string; name: string; value?: string; depth: number }[] = [];
	const links: { source: string; target: string; name: string }[] = [];
	function visit(node: HierarchyNode, depth: number) {
		nodes.push({ id: node.id, name: node.name, value: node.value, depth });
		for (const child of node.children ?? []) {
			links.push({ source: node.id, target: child.id, name: child.value || child.name });
			visit(child, depth + 1);
		}
	}
	visit(root, 0);
	return { nodes, links };
}

export function buildNavigator(root: HierarchyNode, mode: ViewMode) {
	const nodes = [{ id: root.id, parentId: '', x: 100, y: 65, root: true }];
	const vertical = mode === 'graph' || mode === 'top-bottom' || mode === 'bottom-top';
	const reversed = mode === 'right-left' || mode === 'bottom-top';
	// The two branches occupy separate halves of the cross axis.
	function position(depth: number, spread: number) {
		const along = reversed ? 112 - depth * 47 : 18 + depth * 47;
		return vertical ? { x: spread, y: along } : { x: along * 200 / 130, y: spread * 130 / 200 };
	}
	Object.assign(nodes[0], position(0, 100));
	for (const [index, group] of (root.children ?? []).entries()) {
		const start = 12 + index * 96;
		nodes.push({ id: group.id, parentId: root.id, ...position(1, start + 40), root: false });
		const children = group.children ?? [];
		for (const [childIndex, child] of children.entries()) {
			const spread = children.length <= 1 ? start + 40 : start + childIndex / (children.length - 1) * 80;
			nodes.push({ id: child.id, parentId: group.id, ...position(2, spread), root: false });
		}
	}
	return nodes;
}
