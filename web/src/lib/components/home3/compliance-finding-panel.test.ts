import test from 'node:test';
import assert from 'node:assert/strict';

import {
	cacheDetails,
	cachedDetails,
	complianceSearchCounts,
	detailsCacheKey,
	dropFindingDetails,
	verdictCounts,
	withoutActivityLines
} from './compliance-finding-panel.js';
import type { FindingDetails } from '$lib/services/docReviewService';

const counts = { activities: 2, search_results: 64, selected: 5, relevant: 2, judged: 2, verdicts: { unqualified: 1, compatible: 1 } };

test('search counts come from the stored counters', () => {
	assert.deepEqual(complianceSearchCounts(counts), { searched: 64, selected: 5, relevant: 2, irrelevant: 3 });
	assert.equal(complianceSearchCounts(undefined), null);
});

test('verdict counts are in display order and skip absent verdicts', () => {
	assert.deepEqual(verdictCounts(counts), [
		{ verdict: 'compatible', count: 1 },
		{ verdict: 'unqualified', count: 1 }
	]);
	assert.deepEqual(verdictCounts(null), []);
});

test('removes the activity lines from a legacy description', () => {
	const stored =
		'No candidate was left after the knowledge-base search and the relevance check. This does not mean the activity is unregulated.\n\n' +
		'Activity a1 [text, lines 49-52]: document: heat pump water heaters\n' +
		'Activity a2 [metric, lines 60-61]: noise level\n';
	assert.equal(
		withoutActivityLines(stored),
		'No candidate was left after the knowledge-base search and the relevance check. This does not mean the activity is unregulated.'
	);
});

test('details are cached per finding, language, filter and status, and dropped per finding', () => {
	const d = { finding: { id: 7 } } as unknown as FindingDetails;
	const key = detailsCacheKey(7, 'zh-cn', 'judged', 'pending');
	cacheDetails(key, d);
	assert.equal(cachedDetails(key), d);
	assert.equal(cachedDetails(detailsCacheKey(7, 'zh-cn', 'judged', 'accepted')), undefined);
	dropFindingDetails(7);
	assert.equal(cachedDetails(key), undefined);
});
