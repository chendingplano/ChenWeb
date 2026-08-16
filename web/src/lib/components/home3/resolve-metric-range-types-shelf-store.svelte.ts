// Bridges the Value Range Type Map ("Map Block") across the content/shelf
// boundary: the Resolve Metric Range Types page publishes here while it is
// mounted, and the page-level context shelf renders/edits the governed
// kb.metric_value_range_type_map table from it -- mirrors finding-shelf-store's
// precedent for this exact content/shelf pattern.
import {
	applyValueRangeTypeMapEntryToMetrics,
	listValueRangeTypeMapEntries,
	upsertValueRangeTypeMapEntry,
	type ValueRangeTypeMapEntry
} from './resolve-metric-range-types-client.js';

export const rangeTypeMapShelf = $state<{
	active: boolean;
	entries: ValueRangeTypeMapEntry[];
	loading: boolean;
	error: string;
	// Lets the main view refresh its errored-metrics list after a shelf correction.
	onCorrected: (() => void) | null;
}>({
	active: false,
	entries: [],
	loading: false,
	error: '',
	onCorrected: null
});

export async function loadRangeTypeMapEntries() {
	rangeTypeMapShelf.loading = true;
	rangeTypeMapShelf.error = '';
	try {
		const res = await listValueRangeTypeMapEntries();
		rangeTypeMapShelf.entries = res.results;
	} catch (e) {
		rangeTypeMapShelf.error = e instanceof Error ? e.message : String(e);
	} finally {
		rangeTypeMapShelf.loading = false;
	}
}

export async function applyRangeTypeMapEntry(rawValue: string, canonicalBucket: string) {
	const res = await upsertValueRangeTypeMapEntry({
		raw_value: rawValue,
		canonical_bucket: canonicalBucket
	});
	await loadRangeTypeMapEntries();
	rangeTypeMapShelf.onCorrected?.();
	return res;
}

/**
 * Rewrites kb.metrics.value_range_type to an approved entry's canonical_bucket
 * (and clears the error) on every row carrying that raw value. Reloads the map
 * and refreshes the page's errored-metrics list, since applied rows drop out of
 * it.
 */
export async function applyRangeTypeMapEntryToMetrics(rawValue: string) {
	const res = await applyValueRangeTypeMapEntryToMetrics(rawValue);
	await loadRangeTypeMapEntries();
	rangeTypeMapShelf.onCorrected?.();
	return res;
}
