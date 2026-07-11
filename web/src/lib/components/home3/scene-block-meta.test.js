import test from 'node:test';
import assert from 'node:assert/strict';

import { buildSceneBlockMetaSections } from './scene-block-meta.js';

test('buildSceneBlockMetaSections adds english rows only when values differ', () => {
	const sections = buildSceneBlockMetaSections({
		title: '规范性引用文件的应用',
		title_en: 'Application of normative references',
		summary: '标准中规范性引用文件的应用方式。',
		summary_en: 'How normative references are applied in the standard.',
		keywords: ['引用', '标准'],
		keywords_en: ['references', 'standard'],
		states: ['有效'],
		states_en: ['active']
	});

	assert.deepEqual(
		sections.map((section) => section.label),
		['Summary', 'Summary (English)', 'Keywords', 'Keywords (English)', 'STATES', 'STATES (ENGLISH)']
	);
	const [summarySection, summaryEnSection, keywordsSection, keywordsEnSection, statesSection, statesEnSection] = sections;
	assert.ok(summarySection);
	assert.ok(summaryEnSection);
	assert.ok(keywordsSection);
	assert.ok(keywordsEnSection);
	assert.ok(statesSection);
	assert.ok(statesEnSection);
	assert.equal(summarySection.kind, 'text');
	assert.equal(summarySection.value, '标准中规范性引用文件的应用方式。');
	assert.equal(keywordsSection.kind, 'chips');
	assert.equal(keywordsEnSection.kind, 'chips');
	assert.equal(statesSection.kind, 'lines');
	assert.equal(statesEnSection.kind, 'lines');
	assert.deepEqual(keywordsSection.items, ['引用', '标准']);
	assert.deepEqual(keywordsEnSection.items, ['references', 'standard']);
	assert.deepEqual(statesSection.items, ['有效']);
	assert.deepEqual(statesEnSection.items, ['active']);
});

test('buildSceneBlockMetaSections suppresses duplicated english rows', () => {
	const sections = buildSceneBlockMetaSections({
		title: 'Normative Reference Application',
		title_en: 'Normative Reference Application',
		summary: 'Same summary',
		summary_en: 'Same summary',
		keywords: ['same'],
		keywords_en: ['same'],
		states: ['steady'],
		states_en: ['steady']
	});

	assert.deepEqual(
		sections.map((section) => section.label),
		['Summary', 'Keywords', 'STATES']
	);
});
