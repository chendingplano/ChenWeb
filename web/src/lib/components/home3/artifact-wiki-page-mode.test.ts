import test from 'node:test';
import assert from 'node:assert/strict';

import { usesLegacyMetricWikiLayout } from './artifact-wiki-page-mode.js';

test('generic artifact wiki route does not keep a metric-only left-panel branch', () => {
	assert.equal(usesLegacyMetricWikiLayout('metric'), false);
});

test('does not use legacy metric wiki layout for non-metric artifacts', () => {
	assert.equal(usesLegacyMetricWikiLayout('topic'), false);
	assert.equal(usesLegacyMetricWikiLayout('summary'), false);
});
