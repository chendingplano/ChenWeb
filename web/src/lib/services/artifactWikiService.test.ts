import test from 'node:test';
import assert from 'node:assert/strict';

import { buildArtifactWikiApiUrl, buildArtifactWikiHref } from './artifactWikiService.js';

test('buildArtifactWikiHref creates the generic artifact wiki route', () => {
	assert.equal(
		buildArtifactWikiHref({
			artifactType: 'metric',
			artifactId: '5_mtc_3',
			lang: 'en',
			darkMode: true
		}),
		'/home3/knowledge?section=kb-artifact-wiki&artifact_type=metric&artifact_id=5_mtc_3&lang=en'
	);
});

test('buildArtifactWikiHref preserves light mode via dark=0', () => {
	assert.equal(
		buildArtifactWikiHref({
			artifactType: 'topic',
			artifactId: '7_tpc_1',
			lang: 'zh-cn',
			darkMode: false
		}),
		'/home3/knowledge?section=kb-artifact-wiki&artifact_type=topic&artifact_id=7_tpc_1&lang=zh-cn&dark=0'
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
