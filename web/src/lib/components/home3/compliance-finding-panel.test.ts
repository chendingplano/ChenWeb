import test from 'node:test';
import assert from 'node:assert/strict';

import { complianceActivities, complianceSearchCounts, withoutActivityLines } from './compliance-finding-panel.js';

const results = (n: number) => Array.from({ length: n }, (_, i) => ({ artifact_type: 'provision', artifact_id: `9_prv_${i}` }));

// The shape the server's compliance reviewer stores in metadata.artifact_fields (version 3).
const metadata = {
	artifact_fields: {
		version: 3,
		activities: [
			{ activity_id: 'a1', kind: 'text', label: 'document: heat pump water heaters', line_start: 49, line_end: 52 },
			{ activity_id: 'a2', kind: 'metric', label: 'noise level', line_start: 60, line_end: 61 }
		],
		retrieval_trace: [
			{ purpose: 'activity', results: results(30) },
			{ purpose: 'activity', results: results(30) },
			{ purpose: 'cited_document', results: results(4) }
		],
		relevance: [
			{ activity_id: 'a1', artifact_key: 'provision/9_prv_0', kept: true },
			{ activity_id: 'a1', artifact_key: 'provision/9_prv_1', kept: false },
			{ activity_id: 'a2', artifact_key: 'provision/9_prv_2', kept: true },
			{ activity_id: 'a2', artifact_key: 'provision/9_prv_3', kept: false },
			{ activity_id: 'a2', artifact_key: 'provision/9_prv_4', kept: false }
		]
	}
};

test('reads the activities', () => {
	assert.deepEqual(complianceActivities(metadata), [
		{ label: 'document: heat pump water heaters', kind: 'text', lineStart: 49, lineEnd: 52 },
		{ label: 'noise level', kind: 'metric', lineStart: 60, lineEnd: 61 }
	]);
});

test('counts returned entries over every search and the relevance decisions', () => {
	assert.deepEqual(complianceSearchCounts(metadata), { searched: 64, selected: 5, relevant: 2, irrelevant: 3 });
});

test('reads artifact_fields stored as a JSON string', () => {
	const asString = { artifact_fields: JSON.stringify(metadata.artifact_fields) };
	assert.equal(complianceActivities(asString).length, 2);
	assert.equal(complianceSearchCounts(asString)?.searched, 64);
});

test('older findings without these fields give nothing', () => {
	assert.deepEqual(complianceActivities({}), []);
	assert.equal(complianceSearchCounts({ artifact_fields: { activities: [] } }), null);
	assert.equal(complianceSearchCounts(undefined), null);
});

test('removes the activity lines from a description', () => {
	const stored =
		'No candidate was left after the knowledge-base search and the relevance check. This does not mean the activity is unregulated.\n\n' +
		'Activity a1 [text, lines 49-52]: document: heat pump water heaters\n' +
		'Activity a2 [metric, lines 60-61]: noise level\n';
	assert.equal(
		withoutActivityLines(stored),
		'No candidate was left after the knowledge-base search and the relevance check. This does not mean the activity is unregulated.'
	);
});
