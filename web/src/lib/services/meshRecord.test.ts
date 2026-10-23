import test from 'node:test';
import assert from 'node:assert/strict';
import { buildRecordDetails } from './meshRecord';
import type { MeshRecord } from './meshService';

test('details combine preferred concept notes and entry terms without mixing tab data', () => {
	const record: MeshRecord = {
		entity: {
			id: 'D000229',
			label: 'Dependovirus',
			kind: 'TopicalDescriptor',
			properties: [
				{ predicate: 'dateEstablished', values: [{ text: '1979-01-01' }] },
				{ predicate: 'treeNumber', values: [{ id: 'B04.280', label: 'B04.280' }] },
				{ predicate: 'concept', values: [{ id: 'M2' }] },
				{ predicate: 'allowableQualifier', values: [{ id: 'Q1' }] }
			]
		},
		release: '2026',
		qualifiers: [],
		trees: [],
		concepts: [
			{
				preferred: true,
				entity: {
					id: 'M1',
					label: 'Dependovirus',
					kind: 'Concept',
					properties: [{ predicate: 'scopeNote', values: [{ text: 'Preferred scope' }] }]
				},
				terms: [
					{ id: 'T1', kind: 'Term', label: 'Dependovirus', properties: [] },
					{ id: 'T2', kind: 'Term', label: 'Entry term', properties: [] }
				]
			},
			{
				preferred: false,
				entity: {
					id: 'M2',
					kind: 'Concept',
					label: 'Other concept',
					properties: [{ predicate: 'scopeNote', values: [{ text: 'Other scope' }] }]
				},
				terms: [
					{ id: 'T3', kind: 'Term', label: 'Entry term', properties: [] },
					{ id: 'T4', kind: 'Term', label: 'Another entry', properties: [] }
				]
			}
		]
	};
	const rows = buildRecordDetails(record);
	assert.equal(rows[0].predicate, 'heading');
	assert.deepEqual(rows.find((r) => r.predicate === 'scopeNote')?.values, [
		{ text: 'Preferred scope' }
	]);
	assert.deepEqual(rows.find((r) => r.predicate === 'entryTerms')?.values, [
		{ text: 'Another entry' },
		{ text: 'Entry term' }
	]);
	assert.equal(rows.find((r) => r.predicate === 'dateEstablished')?.values[0].text, '1979/01/01');
	assert.equal(
		rows.some((r) => ['concept', 'allowableQualifier'].includes(r.predicate)),
		false
	);
	assert.equal(
		rows.find((r) => r.predicate === 'rdfIdentifier')?.values[0].text,
		'https://id.nlm.nih.gov/mesh/D000229'
	);
});
