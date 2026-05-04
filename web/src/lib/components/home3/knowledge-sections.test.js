import test from 'node:test';
import assert from 'node:assert/strict';

import {
	KNOWLEDGE_UNDER_CONSTRUCTION_SECTIONS,
	isUnderConstructionKnowledgeSection
} from './knowledge-sections.js';

test('knowledge page exposes requested under construction menu sections', () => {
	assert.deepEqual(
		KNOWLEDGE_UNDER_CONSTRUCTION_SECTIONS.map((section) => section.label),
		[
			'References',
			'Formulas',
			'Tables',
			'Compliance Provisions and Terms',
			'Quotations',
			'Case Studies',
			'Workflow',
			'Product and Parts'
		]
	);
});

test('identifies under construction knowledge sections by id', () => {
	assert.equal(isUnderConstructionKnowledgeSection('kb-references'), true);
	assert.equal(isUnderConstructionKnowledgeSection('kb-product-parts'), true);
	assert.equal(isUnderConstructionKnowledgeSection('kb-search'), false);
});
