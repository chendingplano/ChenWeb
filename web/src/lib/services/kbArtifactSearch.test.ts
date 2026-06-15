import test from 'node:test';
import assert from 'node:assert/strict';

import { buildKbArtifactSearchUrl } from './kbArtifactSearch.js';
import { kbSearchArtifactOptions } from '$lib/components/home3/kb-search-lab-state';

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

test('change 02 scopes map to the expected registry artifact types', () => {
	assert.equal(
		buildKbArtifactSearchUrl('semantic-projections', {
			q: 'data return',
			page: 1,
			pageSize: 20
		}),
		'/api/v1/kb/search?q=data+return&page=1&page_size=20&artifact_types=semantic_projection'
	);

	assert.equal(
		buildKbArtifactSearchUrl('entities', {
			q: 'data return',
			page: 1,
			pageSize: 20
		}),
		'/api/v1/kb/search?q=data+return&page=1&page_size=20&artifact_types=entity'
	);

	assert.equal(
		buildKbArtifactSearchUrl('relations', {
			q: 'data return',
			page: 1,
			pageSize: 20
		}),
		'/api/v1/kb/search?q=data+return&page=1&page_size=20&artifact_types=relation'
	);

	assert.equal(
		buildKbArtifactSearchUrl('content-segments', {
			q: 'data return',
			page: 1,
			pageSize: 20
		}),
		'/api/v1/kb/search?q=data+return&page=1&page_size=20&artifact_types=chunk'
	);
});

test('change 02 search chips remove products and expose the new artifact families', () => {
	const labels = kbSearchArtifactOptions.map((option) => option.label);

	assert.deepEqual(labels, [
		'All Artifacts',
		'Metrics',
		'Summaries',
		'Topics',
		'Content Segments',
		'Semantic Projections',
		'Entities',
		'Relations',
		'Scene Blocks',
		'Provisions'
	]);
	assert.equal(labels.includes('Products'), false);
});
