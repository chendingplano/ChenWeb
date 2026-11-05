import test from 'node:test';
import assert from 'node:assert/strict';
import type { Node } from '@xyflow/svelte';
import { componentTree } from './wpi-json';

const labels = { app: 'App', page: 'Page', pageLayouter: 'Page layouter', elementLayouter: 'Element Layouter', elements: 'Elements' };
const element = (id: string, pane: string, x: number, y: number, name: string): Node => ({
	id, type: 'wpi-element', position: { x, y }, data: { label: name, element: name, attrs: {}, pane }
});

test('the component tree lists App, Page, Page layouter with nested panes, Element Layouter and Elements', () => {
	const page = {
		page_layouter: { layouter: 'flow', root: { pane_id: 'main', split: 'rows', children: [{ pane_id: 'top' }, { pane_id: 'bottom', split: 'columns', children: [{ pane_id: 'left' }, { pane_id: 'right' }] }] } },
		panels: { top: { elem_layouter: { layouter: 'flow-layouter' } } }
	};
	const nodes = [element('a', 'top', 0, 0, 'button'), element('b', 'top', 200, 0, 'input'), element('c', 'right', 0, 0, 'text')];
	const tree = componentTree(nodes, page, labels);

	const [page1] = tree.children;
	assert.equal(tree.label, 'App');
	assert.equal(page1.label, 'Page');
	const [pageLayouter, elementLayouter, elements] = page1.children;
	assert.deepEqual(pageLayouter.children.map(n => n.label), ['main (rows)']);
	const main = pageLayouter.children[0];
	assert.deepEqual(main.children.map(n => n.label), ['top', 'bottom (columns)']);
	assert.deepEqual(main.children[1].children.map(n => n.label), ['left', 'right']);
	assert.deepEqual(elementLayouter.children.map(n => n.label), ['top: flow-layouter', 'left', 'right: flow-layouter']);
	assert.deepEqual(elements.children.map(n => n.label), ['element:button', 'element:input', 'element:text']);
	assert.deepEqual(elements.children.map(n => n.target), [{ kind: 'element', node: 'a' }, { kind: 'element', node: 'b' }, { kind: 'element', node: 'c' }]);
});
