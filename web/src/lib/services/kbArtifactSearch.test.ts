import test from 'node:test';
import assert from 'node:assert/strict';

import {
	buildKbArtifactSearchUrl,
	buildKbSearchPageHref,
	matchesKbSearchHistorySnapshot,
	parseKbSearchArtifactType
} from './kbArtifactSearch.js';
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

test('search page href preserves non-default artifact scope', () => {
	assert.equal(
		buildKbSearchPageHref({
			q: '人工智能',
			page: 3,
			artifactType: 'metrics',
			darkMode: false
		}),
		'/home3/knowledge?section=kb-search&q=%E4%BA%BA%E5%B7%A5%E6%99%BA%E8%83%BD&page=3&scope=metrics&dark=0'
	);
});

test('search page href omits default scope and first page noise', () => {
	assert.equal(
		buildKbSearchPageHref({
			q: 'battery',
			page: 1,
			artifactType: 'all',
			darkMode: true
		}),
		'/home3/knowledge?section=kb-search&q=battery'
	);
});

test('invalid artifact scopes fall back to all', () => {
	assert.equal(parseKbSearchArtifactType('metrics'), 'metrics');
	assert.equal(parseKbSearchArtifactType('unknown-scope'), 'all');
	assert.equal(parseKbSearchArtifactType(''), 'all');
});

test('history snapshot only restores when route state matches', () => {
	const snapshot = {
		query: '人工智能',
		submittedQuery: '人工智能',
		artifactType: 'metrics',
		pageNumber: 2,
		currentLocale: 'zh-cn',
		error: '',
		payload: {
			status: true,
			query: '人工智能',
			total: 1,
			results: [{ artifact_id: '1', artifact_type: 'metric', primary_label: 'foo' }]
		}
	};

	assert.equal(
		matchesKbSearchHistorySnapshot(snapshot, {
			query: '人工智能',
			pageNumber: 2,
			artifactType: 'metrics'
		}),
		true
	);
	assert.equal(
		matchesKbSearchHistorySnapshot(snapshot, {
			query: '人工智能',
			pageNumber: 2,
			artifactType: 'all'
		}),
		false
	);
});
