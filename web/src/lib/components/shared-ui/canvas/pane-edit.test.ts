import test from 'node:test';
import assert from 'node:assert/strict';
import type { Node } from '@xyflow/svelte';
import type { JsonObject } from '$lib/wpi/types';
import { addPane, deletePane, moveNode, nodesUnder, renamePane, renameProblem, rootOf, setPageLayouter, setPaneAttr, setPanelConfig, splitPane } from './pane-edit';
import { pageJson } from './wpi-json';

const node = (id: string, pane: string, y = 0): Node => ({ id, type: 'wpi-element', position: { x: 0, y }, data: { label: id, element: 'text', attrs: { text: id }, pane } });

const single: JsonObject = {
	descriptor_id: 'p',
	schema_version: '0.5',
	page_layouter: { layouter: 'single-pane', class: { descriptor_id: 'single-pane', root: { pane_id: 'main' } } },
	panels: { main: { elem_layouter: { layouter: 'flow-layouter', config: { column_gap: '1rem' } } } }
};

test('splitting a leaf moves its panel, elements and leaf attributes to the first child', () => {
	const page = setPaneAttr(setPaneAttr(single, 'main', 'left_margin', '8px'), 'main', 'width', '400px');
	const { page: split, nodes } = splitPane({ page, nodes: [node('a', 'main'), node('b', 'main')] }, 'main', 'columns');
	assert.deepEqual(rootOf(split), { pane_id: 'main', width: '400px', split: 'columns', children: [{ pane_id: 'main-1', left_margin: '8px' }, { pane_id: 'main-2' }] });
	assert.deepEqual(Object.keys(split.panels as JsonObject), ['main-1']);
	assert.deepEqual(nodes.map((n) => n.data.pane), ['main-1', 'main-1']);
	assert.equal(splitPane({ page: split, nodes }, 'main', 'rows').page, split, 'a split pane is not split again');
});

test('adding a pane to a split pane, and pane attributes', () => {
	const { page } = splitPane({ page: single, nodes: [] }, 'main', 'rows');
	const added = addPane(page, 'main');
	assert.deepEqual(rootOf(added).children?.map((c) => c.pane_id), ['main-1', 'main-2', 'main-3']);
	assert.equal(addPane(added, 'main-1'), added, 'a leaf gets no children');
	assert.equal(rootOf(setPaneAttr(added, 'main-3', 'height', '48px')).children?.[2].height, '48px');
	assert.equal(rootOf(setPaneAttr(setPaneAttr(added, 'main-3', 'height', '48px'), 'main-3', 'height', '')).children?.[2].height, undefined);
});

test('deleting a pane removes its elements; a split pane left with one child becomes that child', () => {
	const { page, nodes } = splitPane({ page: setPaneAttr(single, 'main', 'width', '400px'), nodes: [node('a', 'main')] }, 'main', 'columns');
	const withB = [...nodes, node('b', 'main-2')];
	assert.deepEqual(nodesUnder(page, withB, 'main-2').map((n) => n.id), ['b']);
	const after = deletePane({ page, nodes: withB }, 'main-2');
	assert.deepEqual(rootOf(after.page), { pane_id: 'main-1', width: '400px' }, 'the split pane\'s sizes stay');
	assert.deepEqual(after.nodes.map((n) => n.id), ['a']);
	assert.equal(rootOf(page).children?.length, 2, 'the page before is unchanged');
	assert.equal(deletePane(after, 'main-1').page, after.page, 'the root cannot be deleted');
});

test('renaming a pane moves its panel and elements; IDs are checked', () => {
	const r = renamePane({ page: single, nodes: [node('a', 'main')] }, 'main', 'body');
	assert.equal(rootOf(r.page).pane_id, 'body');
	assert.deepEqual(Object.keys(r.page.panels as JsonObject), ['body']);
	assert.equal(r.nodes[0].data.pane, 'body');
	assert.equal(renameProblem(r.page, 'body', 'Body'), 'invalid');
	const { page } = splitPane({ page: r.page, nodes: [] }, 'body', 'rows');
	assert.equal(renameProblem(page, 'body-1', 'body-2'), 'taken');
	assert.equal(renameProblem(page, 'body-1', 'body-1'), null);
});

test('moving an element puts it at the end of the other pane', () => {
	const nodes = moveNode([node('a', 'left', 0), node('b', 'right', 100), node('c', 'right', 200)], 'a', 'right');
	const page = { ...single, page_layouter: { layouter: 'x', root: { pane_id: 'r', split: 'columns', children: [{ pane_id: 'left' }, { pane_id: 'right' }] } } };
	assert.deepEqual(((pageJson(nodes, page).panels as JsonObject).right as { elem_list: JsonObject[] }).elem_list.map((e) => e.text), ['b', 'c', 'a']);
});

test('a panel config replaces the copy\'s; an empty one clears it', () => {
	const set = setPanelConfig(single, 'main', { row_gap: '2rem' });
	assert.deepEqual((set.panels as Record<string, { elem_layouter: JsonObject }>).main.elem_layouter, { layouter: 'flow-layouter', config: { row_gap: '2rem' } });
	assert.deepEqual((setPanelConfig(set, 'main', {}).panels as Record<string, { elem_layouter: JsonObject }>).main.elem_layouter, { layouter: 'flow-layouter' });
	assert.ok((setPanelConfig(single, 'other', { row_gap: '1rem' }).panels as JsonObject).other, 'a panel is created for a leaf without one');
});

test('changing the page layouter preserves the current pane tree and panels', () => {
 const cls: JsonObject = { descriptor_id: 'other', root: { pane_id: 'new' } };
 const changed = setPageLayouter(single, cls);
 assert.deepEqual(changed.page_layouter, { layouter: 'other', class: cls, root: { pane_id: 'main' } });
 assert.deepEqual(changed.panels, single.panels);
 assert.deepEqual(rootOf(single), { pane_id: 'main' });
});
