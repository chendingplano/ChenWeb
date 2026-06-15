import test from 'node:test';
import assert from 'node:assert/strict';

import { metricWikiCopyForLang } from './metric-wiki-i18n.js';

test('metricWikiCopyForLang returns Chinese copy for zh-cn', () => {
	const copy = metricWikiCopyForLang('zh-cn');
	assert.equal(copy.definition, '定义');
	assert.equal(copy.artifactRecord, '制品记录');
});

test('metricWikiCopyForLang defaults to English copy', () => {
	const copy = metricWikiCopyForLang('en');
	assert.equal(copy.definition, 'Definition');
	assert.equal(copy.relatedMetrics, 'Related metrics');
});
