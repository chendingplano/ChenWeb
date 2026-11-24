// What the Finding panel and the report list show of a compliance finding. Its summary
// carries stored counters; its activities, candidates and search history load on demand
// (openspec compliance-review-storage).

import type { ComplianceCounts, FindingDetails } from '$lib/services/docReviewService';

export type ComplianceSearchCounts = {
	searched: number; // entries returned, summed over every search of the finding
	selected: number; // activity–candidate pairs sent to the relevance check
	relevant: number; // pairs the relevance check kept
	irrelevant: number; // pairs it removed
};

// The display order of verdicts: actionable first, droppable last.
export const VERDICT_ORDER = ['not_compliant', 'compatible', 'compliant', 'undetermined', 'unqualified'] as const;

export function complianceSearchCounts(counts: ComplianceCounts | undefined | null): ComplianceSearchCounts | null {
	if (!counts) return null;
	return { searched: counts.search_results, selected: counts.selected, relevant: counts.relevant, irrelevant: counts.selected - counts.relevant };
}

// The verdicts a finding's judged candidates received, with counts, in display order.
export function verdictCounts(counts: ComplianceCounts | undefined | null): { verdict: string; count: number }[] {
	const v = counts?.verdicts ?? {};
	return VERDICT_ORDER.filter((k) => (v[k] ?? 0) > 0).map((k) => ({ verdict: k, count: v[k] }));
}

const ACTIVITY_LINE = /^Activity \S+ \[[^\],]+, lines \d+-\d+\]: /;

// Activity labels may be stored as slugs; keep the stored identifiers intact.
export function readableActivityLabel(label: string): string {
	return label.replace(/_/g, ' ');
}

// A legacy description without its "Activity a1 [text, lines 49-52]: ..." lines.
export function withoutActivityLines(description: string): string {
	return description
		.split('\n')
		.filter((line) => !ACTIVITY_LINE.test(line))
		.join('\n')
		.trim();
}

// Loaded details are kept per finding, language, candidate filter and review status, so a
// review action (which changes the status) loads them again.
const MAX_CACHED = 50;
const detailsCache = new Map<string, FindingDetails>();

export function detailsCacheKey(id: number, language: string, filter: string, reviewStatus: string): string {
	return `${id}|${language}|${filter}|${reviewStatus}`;
}

export function cachedDetails(key: string): FindingDetails | undefined {
	const d = detailsCache.get(key);
	if (d) {
		detailsCache.delete(key);
		detailsCache.set(key, d); // most recently used last
	}
	return d;
}

export function cacheDetails(key: string, d: FindingDetails): void {
	detailsCache.set(key, d);
	while (detailsCache.size > MAX_CACHED) {
		const oldest = detailsCache.keys().next().value;
		if (oldest === undefined) break;
		detailsCache.delete(oldest);
	}
}

// dropFindingDetails forgets every cached entry of one finding.
export function dropFindingDetails(id: number): void {
	for (const key of [...detailsCache.keys()]) if (key.startsWith(`${id}|`)) detailsCache.delete(key);
}
