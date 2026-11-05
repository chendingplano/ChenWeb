import test from 'node:test';
import assert from 'node:assert/strict';
import type { Node } from '@xyflow/svelte';
import { componentTree, runtimeComponentTree } from './wpi-json';
import type { Bundle } from '$lib/wpi/types';

const labels = { app: 'App', page: 'Page', pageLayouter: 'Page Layouter', paneTree: 'Pane Tree', headlessPage: 'Headless Page', compoundPage: 'Compound Page', elementLayouter: 'Element Layouter', elements: 'Element List' };
const element = (id: string, pane: string, x: number, y: number, name: string): Node => ({
	id, type: 'wpi-element', position: { x, y }, data: { label: name, element: name, attrs: {}, pane }
});

test('each leaf pane owns its Headless Page, layouter and element list', () => {
	const page = {
		page_layouter: { layouter: 'flow', root: { pane_id: 'main', split: 'rows', children: [{ pane_id: 'top' }, { pane_id: 'bottom', split: 'columns', children: [{ pane_id: 'left' }, { pane_id: 'right' }] }] } },
		panels: { top: { elem_layouter: { layouter: 'flow-layouter' } } }
	};
	const nodes = [element('a', 'top', 0, 0, 'button'), element('b', 'top', 200, 0, 'input'), element('c', 'right', 0, 0, 'text')];
	nodes[0].data.instance_id = 'button_6_4';
	const tree = componentTree(nodes, page, labels);

	const [page1] = tree.children;
	assert.equal(tree.label, 'App');
	assert.equal(page1.label, 'Page');
	assert.equal(tree.children.length, 1);
	assert.equal(page1.children.length, 1);
	const [pageLayouter] = page1.children;
	assert.deepEqual(pageLayouter.children.map(n => n.label), ['Pane Tree']);
	assert.deepEqual(pageLayouter.target, { kind: 'page-layouter' });
	const main = pageLayouter.children[0].children[0];
	assert.deepEqual(main.target, { kind: 'pane', pane: 'main' });
	assert.deepEqual(main.children.map(n => n.label), ['top', 'bottom (columns)']);
	assert.deepEqual(main.children[1].children.map(n => n.label), ['left', 'right']);
	const [top, bottom] = main.children;
	const [headless] = top.children;
	assert.equal(top.children.length, 1);
	assert.equal(headless.label, 'Headless Page');
	assert.deepEqual(headless.target, { kind: 'panel', pane: 'top' });
	assert.deepEqual(headless.children.map(n => n.label), ['Element Layouter: flow-layouter', 'Element List']);
	assert.deepEqual(headless.children[0].target, { kind: 'panel', pane: 'top' });
	const elements = headless.children[1];
	assert.deepEqual(elements.children.map(n => n.label), ['button:button_6_4', 'input']);
	assert.deepEqual(elements.children.map(n => n.target), [{ kind: 'element', node: 'a' }, { kind: 'element', node: 'b' }]);
	const [left, right] = bottom.children;
	assert.deepEqual(left.children[0].children[1].children, []);
	assert.deepEqual(right.children[0].children[1].children.map(n => n.target), [{ kind: 'element', node: 'c' }]);
});

test('a blank single-pane page still shows an empty Headless Page with exactly two children', () => {
	const tree = componentTree([], {}, labels);
	const root = tree.children[0].children[0].children[0].children[0];
	assert.equal(root.label, 'main');
	assert.equal(root.children.length, 1);
	assert.deepEqual(root.children[0].children.map(n => n.label), ['Element Layouter: flow-layouter', 'Element List']);
	assert.deepEqual(root.children[0].children[1].children, []);
});

test('pane ownership and reading order match saved JSON, including orphaned nodes and value editors', () => {
	const nodes = [element('last', 'main', 200, 100, 'button'), element('editor', 'main', 0, 100, 'string-editor'), element('orphan', 'gone', 0, 0, 'text')];
	const tree = componentTree(nodes, {}, labels);
	const list = tree.children[0].children[0].children[0].children[0].children[0].children[1];
	assert.deepEqual(list.children.map(n => n.target), [{ kind: 'element', node: 'orphan' }, { kind: 'element', node: 'editor' }, { kind: 'element', node: 'last' }]);
	assert.equal(new Set(list.children.map(n => n.id)).size, 3);
	assert.deepEqual(nodes.map(n => n.data.pane), ['main', 'main', 'gone']);
});

test('preview replaces a leaf content with the visible Compound Page and recurses by instance', () => {
	const bundle = { pages: {
		home: { descriptor_id: 'home', page_layouter: { root: { pane_id: 'main' } }, panels: {} },
		compound: { descriptor_id: 'compound', page_layouter: { root: { pane_id: 'main', split: 'columns', children: [{ pane_id: 'left' }, { pane_id: 'right' }] } }, panels: {} },
		form: { descriptor_id: 'form', page_layouter: { root: { pane_id: 'main' } }, panels: { main: { elem_layouter: { layouter: 'flow-layouter' }, elem_list: [{ element: 'string-editor' }] } } },
	} } as unknown as Pick<Bundle, 'pages'>;
	const state = { bundle, root: 'home-1', pages: { 'home-1': { pageId: 'home' }, 'compound-1': { pageId: 'compound' }, 'form-1': { pageId: 'form' } }, panes: { 'home-1/main': { visible: 'compound-1' as string | null }, 'compound-1/right': { visible: 'form-1' } } };
	const tree = runtimeComponentTree(state, labels)!;
	const host = tree.children[0].children[0].children[0].children[0];
	assert.equal(host.children.length, 1);
	const compound = host.children[0];
	assert.equal(compound.label, 'Compound Page: compound');
	assert.deepEqual(compound.children.map(n => n.label), ['Page Layouter']);
	const split = compound.children[0].children[0].children[0];
	assert.deepEqual(split.children.map(n => n.label), ['left', 'right']);
	const form = split.children[1].children[0];
	assert.equal(form.label, 'Headless Page: form');
	assert.deepEqual(form.children.map(n => n.label), ['Element Layouter: flow-layouter', 'Element List']);
	assert.equal(form.children[1].children[0].label, 'string-editor');
	const all = (node: typeof tree): (typeof tree)[] => [node, ...node.children.flatMap(all)];
	assert.equal(all(tree).every(n => !n.target), true, 'preview does not select nodes of another page in the edit canvas');
	assert.equal(new Set(all(tree).map(n => n.id)).size, all(tree).length);
	state.panes['home-1/main'].visible = null;
	assert.equal(runtimeComponentTree(state, labels)!.children[0].children[0].children[0].children[0].children[0].label, 'Headless Page');
});
