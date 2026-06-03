export type KbSearchPaginationItem = number | 'ellipsis';

export function clampKbSearchPage(page: number, totalPages: number): number {
	if (!Number.isFinite(page)) return 1;
	return Math.min(Math.max(1, Math.trunc(page)), Math.max(1, Math.trunc(totalPages)));
}

export function buildKbSearchPaginationItems(
	currentPage: number,
	totalPages: number
): KbSearchPaginationItem[] {
	const safeTotalPages = Math.max(1, Math.trunc(totalPages));
	const safeCurrentPage = clampKbSearchPage(currentPage, safeTotalPages);

	if (safeTotalPages <= 7) {
		return Array.from({ length: safeTotalPages }, (_, index) => index + 1);
	}

	const items: KbSearchPaginationItem[] = [1];
	const windowStart = Math.max(2, safeCurrentPage - 1);
	const windowEnd = Math.min(safeTotalPages - 1, safeCurrentPage + 1);

	if (windowStart > 2) {
		items.push('ellipsis');
	}

	for (let page = windowStart; page <= windowEnd; page += 1) {
		items.push(page);
	}

	if (windowEnd < safeTotalPages - 1) {
		items.push('ellipsis');
	}

	items.push(safeTotalPages);
	return items;
}
