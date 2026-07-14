import test from 'node:test';
import assert from 'node:assert/strict';

import { buildArtifactWikiApiUrl, buildArtifactWikiHref } from './artifactWikiService.js';

test('buildArtifactWikiHref creates the generic artifact wiki route', () => {
	assert.equal(
		buildArtifactWikiHref({
			artifactType: 'metric',
			artifactId: '5_mtc_3',
			lang: 'en'
		}),
		'/home3/knowledge?section=kb-artifact-wiki&artifact_type=metric&artifact_id=5_mtc_3&lang=en'
	);
});

// Light/dark mode rides on the shared theme store, not the href — see
// $lib/stores/theme.svelte.
test('buildArtifactWikiHref carries no mode parameter', () => {
	assert.equal(
		buildArtifactWikiHref({
			artifactType: 'topic',
			artifactId: '7_tpc_1',
			lang: 'zh-cn'
		}),
		'/home3/knowledge?section=kb-artifact-wiki&artifact_type=topic&artifact_id=7_tpc_1&lang=zh-cn'
	);
});

test('buildArtifactWikiApiUrl can request record-only payloads', () => {
	assert.equal(
		buildArtifactWikiApiUrl({
			artifactType: 'metric',
			artifactId: '5_mtc_3',
			lang: 'zh-cn',
			includeArticle: false
		}),
		'/api/v1/kb/artifacts/wiki?artifact_type=metric&artifact_id=5_mtc_3&lang=zh-cn&include_article=0'
	);
});
