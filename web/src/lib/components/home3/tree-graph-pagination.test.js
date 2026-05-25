import test from 'node:test';
import assert from 'node:assert/strict';

import {
	buildPaginatedChildren,
	nextVirtualId,
	pageVirtualId,
	parseNextVirtualId
} from './tree-graph-pagination.js';

/**
 * @typedef {{ id: string, name: string, children?: TestNode[] }} TestNode
 */

/**
 * @param {string} id
 * @returns {{ id: string, label: string }}
 */
function makeChild(id) {
	return { id, label: id };
}

test('buildPaginatedChildren keeps revealed pages as siblings on the page level', () => {
	const children = Array.from({ length: 65 }, (_, index) => makeChild(`child-${index + 1}`));

	/** @type {TestNode[]} */
	const pageLevel = buildPaginatedChildren({
		parentId: 'biology',
		allChildren: children,
		pageSize: 30,
		revealedPages: 2,
		buildChildNode: (child) => ({ id: child.id, name: child.label }),
		buildPageNode: ({ parentId, pageNum, totalPages, pageChildren }) => ({
			id: pageVirtualId(parentId, pageNum),
			name: `page ${pageNum} of ${totalPages}`,
			children: pageChildren
		}),
		buildNextNode: ({ parentId, pageNum }) => ({
			id: nextVirtualId(parentId, pageNum),
			name: 'next …',
			children: []
		})
	});

	assert.equal(pageLevel.length, 3);
	assert.deepEqual(
		pageLevel.map((node) => node.id),
		[
			pageVirtualId('biology', 1),
			pageVirtualId('biology', 2),
			nextVirtualId('biology', 3)
		]
	);
	assert.equal(pageLevel[0].children.length, 30);
	assert.equal(pageLevel[1].children.length, 30);
	assert.equal(pageLevel[2].children.length, 0);
	assert.equal(pageLevel[0].children.at(-1).id, 'child-30');
	assert.equal(pageLevel[1].children.at(-1).id, 'child-60');
});

test('buildPaginatedChildren returns direct children when pagination is unnecessary', () => {
	const children = Array.from({ length: 3 }, (_, index) => makeChild(`child-${index + 1}`));

	/** @type {TestNode[]} */
	const pageLevel = buildPaginatedChildren({
		parentId: 'biology',
		allChildren: children,
		pageSize: 30,
		revealedPages: 1,
		buildChildNode: (child) => ({ id: child.id, name: child.label }),
		buildPageNode: () => {
			throw new Error('page nodes should not be built');
		},
		buildNextNode: () => {
			throw new Error('next nodes should not be built');
		}
	});

	assert.deepEqual(pageLevel, [
		{ id: 'child-1', name: 'child-1' },
		{ id: 'child-2', name: 'child-2' },
		{ id: 'child-3', name: 'child-3' }
	]);
});

test('parseNextVirtualId extracts the parent id and next page number', () => {
	assert.deepEqual(parseNextVirtualId(nextVirtualId('biology/physics', 4)), {
		parentId: 'biology/physics',
		nextPage: 4
	});
	assert.equal(parseNextVirtualId(pageVirtualId('biology', 2)), null);
});
