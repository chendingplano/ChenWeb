import test from 'node:test';
import assert from 'node:assert/strict';

import {
	filterNodesInSelectedLevel,
	getCategoryPathLevel,
	getNodesInSelectedLevel
} from './tree-graph-filter-state.js';

/**
 * @param {string} id
 * @param {string[]} [keywords]
 * @param {string} [createTime]
 */
function makeNode(id, keywords = [], createTime = '20260501-120000') {
	return {
		id,
		label: id.split('/').at(-1) ?? id,
		categoryPath: id,
		metadata: {
			desc: `${id} description`,
			keywords,
			create_time: createTime
		},
		childIds: []
	};
}

test('getCategoryPathLevel treats root category segments as level 0', () => {
	assert.equal(getCategoryPathLevel('finance'), 0);
	assert.equal(getCategoryPathLevel('finance/tax/federal'), 2);
});

test('getNodesInSelectedLevel returns only nodes at the clicked node level', () => {
	const nodes = [
		makeNode('finance'),
		makeNode('research'),
		makeNode('finance/tax'),
		makeNode('finance/payroll'),
		makeNode('finance/tax/federal')
	];

	const currentLevel = getNodesInSelectedLevel(nodes, 'finance/tax');

	assert.deepEqual(
		currentLevel.map((node) => node.id),
		['finance/tax', 'finance/payroll']
	);
});

test('filterNodesInSelectedLevel applies keyword and timestamp conditions only to the selected level', () => {
	const nodes = [
		makeNode('finance', ['tax'], '20260501-120000'),
		makeNode('finance/tax', ['federal', 'filing'], '20260502-120000'),
		makeNode('finance/payroll', ['payroll'], '20260503-120000'),
		makeNode('research/filing', ['federal', 'filing'], '20260504-120000')
	];

	const matches = filterNodesInSelectedLevel(nodes, 'finance/tax', {
		keywords: 'federal filing',
		startTime: '20260502-000000',
		endTime: '20260503-000000'
	});

	assert.deepEqual(matches, ['finance/tax']);
});
