import test from 'node:test';
import assert from 'node:assert/strict';

import { usesLegacyMetricWikiLayout } from './artifact-wiki-page-mode.js';

test('uses legacy metric wiki layout for metrics', () => {
	assert.equal(usesLegacyMetricWikiLayout('metric'), true);
});

test('does not use legacy metric wiki layout for non-metric artifacts', () => {
	assert.equal(usesLegacyMetricWikiLayout('topic'), false);
	assert.equal(usesLegacyMetricWikiLayout('summary'), false);
});
