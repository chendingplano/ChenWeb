import test from 'node:test';
import assert from 'node:assert/strict';
import type { Node } from '@xyflow/svelte';
import type { JsonObject } from '$lib/wpi/types';
import { addPane, deletePane, mergePane, moveNode, nodesUnder, renamePane, renameProblem, resizePane, rootOf, setPageLayouter, setPaneAttr, setPanelConfig, splitPane } from './pane-edit';
import { nodePane, pageJson } from './wpi-json';

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
	assert.deepEqual(rootOf(split), { pane_id: 'main', width: '400px', left_margin: '8px', split: 'columns', children: [{ pane_id: 'main-1' }, { pane_id: 'main-2' }] });
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

test('changing the page layouter applies its tree and moves displaced content to the first leaf', () => {
 const cls: JsonObject = { descriptor_id: 'other', root: { pane_id: 'page', split: 'rows', children: [{ pane_id: 'top' }, { pane_id: 'bottom' }] } };
 const changed = setPageLayouter({ page: single, nodes: [node('a', 'main')] }, cls);
 assert.deepEqual(rootOf(changed.page), cls.root);
 assert.equal((changed.page.page_layouter as JsonObject).class, cls);
 assert.deepEqual(Object.keys(changed.page.panels as JsonObject), ['top']);
 assert.deepEqual(changed.nodes.map(nodePane), ['top']);
 assert.deepEqual(rootOf(single), { pane_id: 'main' });
});

test('changing layouter retains matching panes and moves only displaced elements', () => {
 const page: JsonObject = { ...single, page_layouter: { layouter: 'x', root: { pane_id: 'page', split: 'rows', children: [{ pane_id: 'left' }, { pane_id: 'right' }] } }, panels: { left: { elem_layouter: { layouter: 'flow-layouter' } }, right: { elem_layouter: { layouter: 'flow-layouter' } } } };
 const cls: JsonObject = { descriptor_id: 'other', root: { pane_id: 'page', split: 'rows', children: [{ pane_id: 'left' }, { pane_id: 'bottom' }] } };
 const result = setPageLayouter({ page, nodes: [node('a', 'left'), node('b', 'right')] }, cls);
 assert.deepEqual(result.nodes.map(nodePane), ['left', 'left']);
 assert.deepEqual(Object.keys(result.page.panels as JsonObject), ['left']);
});


test('merging a split keeps all elements in pane order', () => {
 const { page } = splitPane({ page: single, nodes: [] }, 'main', 'columns');
 const merged = mergePane({ page, nodes: [node('b', 'main-2'), node('a', 'main-1')] }, 'main');
 assert.deepEqual(rootOf(merged.page), { pane_id: 'main' });
 assert.deepEqual(merged.nodes.map(nodePane), ['main', 'main']);
 assert.deepEqual(((pageJson(merged.nodes, merged.page).panels as JsonObject).main as { elem_list: JsonObject[] }).elem_list.map(e => e.text), ['a', 'b']);
});

test('resizing a pane expands only an exceeded maximum', () => {
 const page = setPaneAttr(single, 'main', 'max_width', '100px');
 const resized = resizePane(page, 'main', 'width', 140, 500);
 assert.equal(rootOf(resized).width, '140px');
 assert.equal(rootOf(resized).max_width, '140px');
 assert.equal(rootOf(resizePane(page, 'main', 'width', 80, 500)).max_width, '100px');
 assert.equal(rootOf(resizePane(setPaneAttr(single, 'main', 'max_width', '20%'), 'main', 'width', 140, 500)).max_width, '140px');
});

test('resizing below a pane minimum lowers it, including the default', () => {
 const page = setPaneAttr(single, 'main', 'min_width', '160px');
 assert.equal(rootOf(resizePane(page, 'main', 'width', 80, 500)).min_width, '80px');
 assert.equal(rootOf(resizePane(page, 'main', 'width', 200, 500)).min_width, '160px');
 assert.equal(rootOf(resizePane(single, 'main', 'height', 25, 500)).min_height, '25px');
 const percent = setPaneAttr(single, 'main', 'min_width', '20%');
 assert.equal(rootOf(resizePane(percent, 'main', 'width', 75, 500)).min_width, '75px');
});

test('pane sizes and pixel minimum attributes cannot be set below 20px', () => {
 const page = setPaneAttr(single, 'main', 'min_width', '5px');
 assert.equal(rootOf(page).min_width, '20px');
 assert.equal(rootOf(setPaneAttr(page, 'main', 'min_height', '10px')).min_height, '20px');
 const resized = resizePane(setPaneAttr(single, 'main', 'min_width', '160px'), 'main', 'width', 5, 500);
 assert.equal(rootOf(resized).width, '20px');
 assert.equal(rootOf(resized).min_width, '20px');
});
