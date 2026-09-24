export function selectedPageIds(
	selectedIds: Set<number>,
	visibleIds: number[],
	selectVisible: boolean
): Set<number> {
	const next = new Set(selectedIds);
	for (const id of visibleIds) {
		if (selectVisible) next.add(id);
		else next.delete(id);
	}
	return next;
}

export function togglePageRecord(selectedIds: Set<number>, id: number): Set<number> {
	const next = new Set(selectedIds);
	if (next.has(id)) next.delete(id);
	else next.add(id);
	return next;
}

export function togglePageSelection(selectedIds: Set<number>, visibleIds: number[]): boolean {
	return visibleIds.some((id) => !selectedIds.has(id));
}
