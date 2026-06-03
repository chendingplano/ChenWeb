import test from 'node:test';
import assert from 'node:assert/strict';

import { buildKbArtifactSearchUrl } from './kbArtifactSearch.js';

test('metric scope uses the hybrid registry endpoint with artifact_types filter', () => {
	const url = buildKbArtifactSearchUrl('metrics', {
		q: 'energy intensity',
		page: 2,
		pageSize: 10
	});

	assert.equal(
		url,
		'/api/v1/kb/search?q=energy+intensity&page=2&page_size=10&artifact_types=metric'
	);
});

test('all scope uses the hybrid registry endpoint without artifact_types filter', () => {
	const url = buildKbArtifactSearchUrl('all', {
		q: 'battery',
		page: 1,
		pageSize: 20
	});

	assert.equal(url, '/api/v1/kb/search?q=battery&page=1&page_size=20');
});
