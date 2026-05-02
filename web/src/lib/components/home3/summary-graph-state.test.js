import test from 'node:test';
import assert from 'node:assert/strict';

import {
	createSummaryGraphTabs,
	openCategorySummaryTab,
	toggleNodeExpanded,
	renameNode,
	addChildNode,
	deleteNode
} from './summary-graph-state.js';

/**
 * @param {string} id
 * @param {string} label
 * @param {string[]} [childIds]
 * @param {Record<string, unknown>} [extra]
 */
function makeNode(id, label, childIds = [], extra = {}) {
	return {
		id,
		label,
		categoryPath: id,
		metadata: {
			desc: `${label} description`,
			category_type: 'topic',
			confidence: 0.8,
			keywords: [],
			create_time: '20260501-000000'
		},
		childIds,
		summaryIds: [],
		expanded: false,
		...extra
	};
}

test('summary graph starts with a fixed non-closable graph tab', () => {
	const tabs = createSummaryGraphTabs();

	assert.deepEqual(tabs, [
		{
			id: 'summary-graph',
			label: 'Summary Graph',
			categoryPath: null,
			closable: false
		}
	]);
});

test('opening a category path creates a tab once and then reuses it', () => {
	const initial = createSummaryGraphTabs();
	const firstOpen = openCategorySummaryTab(initial, 'finance/tax');
	const secondOpen = openCategorySummaryTab(firstOpen.tabs, 'finance/tax');

	assert.equal(firstOpen.activeTabId, 'category:finance/tax');
	assert.equal(firstOpen.tabs.length, 2);
	assert.equal(secondOpen.activeTabId, 'category:finance/tax');
	assert.equal(secondOpen.tabs.length, 2);
});

test('toggleNodeExpanded flips the expansion state for one node only', () => {
	const nodes = [makeNode('root', 'Root', ['child']), makeNode('child', 'Child')];

	const next = toggleNodeExpanded(nodes, 'root');

	assert.equal(next[0].expanded, true);
	assert.equal(next[1].expanded, false);
});

test('renameNode updates only the targeted node label', () => {
	const nodes = [makeNode('root', 'Root', ['child']), makeNode('child', 'Child')];

	const next = renameNode(nodes, 'child', 'Renamed Child');

	assert.equal(next[0].label, 'Root');
	assert.equal(next[1].label, 'Renamed Child');
});

test('addChildNode appends a child and updates the parent childIds', () => {
	const nodes = [makeNode('root', 'Root')];

	const next = addChildNode(nodes, 'root', makeNode('child', 'Child'));

	assert.equal(next.length, 2);
	assert.deepEqual(next[0].childIds, ['child']);
	assert.equal(next[1].id, 'child');
});

test('deleteNode removes the node and cleans it from parent childIds', () => {
	const nodes = [makeNode('root', 'Root', ['child']), makeNode('child', 'Child')];

	const next = deleteNode(nodes, 'child');

	assert.equal(next.length, 1);
	assert.deepEqual(next[0].childIds, []);
});
