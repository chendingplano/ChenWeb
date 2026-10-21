type Selection = { itemId: string; childId?: string };
type NavNode = { id: string; label: string; href?: string; children?: NavNode[] };

// Store stable IDs rather than translated titles so a reload uses the new locale's labels.
export function selectionUrl(current: URL, selection: Selection): URL {
	const url = new URL(current);
	url.searchParams.set('view', selection.itemId);
	if (selection.childId) url.searchParams.set('child', selection.childId);
	else url.searchParams.delete('child');
	return url;
}

export function resolveSelection(url: URL, nodes: NavNode[]) {
	const item = nodes.find((node) => node.id === url.searchParams.get('view'));
	if (!item || item.href) return null;
	const childId = url.searchParams.get('child');
	function findChild(children: NavNode[]): NavNode | undefined {
		for (const child of children) {
			if (child.id === childId) return child;
			const found = findChild(child.children ?? []);
			if (found) return found;
		}
	}
	const child = childId ? findChild(item.children ?? []) : undefined;
	if (childId && (!child || child.href)) return null;
	return {
		itemId: item.id,
		itemTitle: item.label,
		childId: child?.id,
		childTitle: child?.label
	};
}
