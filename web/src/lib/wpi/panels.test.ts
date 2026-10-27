import test from 'node:test';
import assert from 'node:assert/strict';
import { isBound, planOpen, removePanel, sharingKey, showPanel, type Bindings, type PaneModel, type PanelModel } from './panels';

function setup() {
	const pane: PaneModel = { panels: [], visible: null };
	const panels: Record<string, PanelModel> = {};
	const bindings: Bindings = {};
	const add = (id: string, pageId: string, binding?: string) => {
		panels[id] = { id, paneId: 'middle', pageId, key: sharingKey(pageId, {}) };
		showPanel(pane, id);
		if (binding) bindings[binding] = id;
	};
	return { pane, panels, bindings, add };
}

test('sharing key depends on page and input, not key order', () => {
	const ref = { kind: 'page', id: 'form', pointer: '/input' } as const;
	assert.equal(sharingKey('form', { ref, args: { a: 1, b: 2 } }), sharingKey('form', { args: { b: 2, a: 1 }, ref }));
	assert.notEqual(sharingKey('form', {}), sharingKey('form', { ref }));
	assert.equal(sharingKey('form', undefined), sharingKey('form', {}));
});

test('an empty pane opens a new panel', () => {
	const { pane, panels, bindings } = setup();
	assert.deepEqual(planOpen(pane, panels, bindings, { key: sharingKey('customers', {}), binding: 'nav/customers' }), { reuse: null, current: null });
});

test('switching back to a menu item reuses its panel and hides the bound one', () => {
	const { pane, panels, bindings, add } = setup();
	add('customers#1', 'customers', 'nav/customers');
	add('form#2', 'customer-form', 'nav/new-customer');
	const plan = planOpen(pane, panels, bindings, { key: sharingKey('customers', {}), binding: 'nav/customers' });
	assert.deepEqual(plan, { reuse: 'customers#1', current: { id: 'form#2', fate: 'hide' } });
});

test('two menu items for the same page and input share one panel', () => {
	const { pane, panels, bindings, add } = setup();
	add('customers#1', 'customers', 'nav/customers');
	const plan = planOpen(pane, panels, bindings, { key: sharingKey('customers', {}), binding: 'nav/all-customers' });
	assert.equal(plan.reuse, 'customers#1');
	assert.equal(plan.current, null);
});

test('an unbound panel is closed when replaced', () => {
	const { pane, panels, bindings, add } = setup();
	add('form#7', 'customer-form');
	const plan = planOpen(pane, panels, bindings, { key: sharingKey('customer-form', { ref: { kind: 'page', id: 'customers', pointer: '/elements/1/on_row_click/input' }, args: { id: 9 } }) });
	assert.deepEqual(plan, { reuse: null, current: { id: 'form#7', fate: 'close' } });
});

test('a non-menu open never reuses a panel', () => {
	const { pane, panels, bindings, add } = setup();
	add('customers#1', 'customers', 'nav/customers');
	assert.equal(planOpen(pane, panels, bindings, { key: sharingKey('customers', {}) }).reuse, null);
});

test('removing the visible panel shows the most recently shown remaining one and drops its bindings', () => {
	const { pane, bindings, add } = setup();
	add('a#1', 'a', 'nav/a');
	add('b#2', 'b', 'nav/b');
	add('c#3', 'c', 'nav/c');
	showPanel(pane, 'a#1');
	assert.deepEqual(pane.panels, ['b#2', 'c#3', 'a#1']);
	assert.equal(removePanel(pane, bindings, 'a#1'), 'c#3');
	assert.equal(pane.visible, 'c#3');
	assert.equal(isBound(bindings, 'a#1'), false);
	assert.equal(removePanel(pane, bindings, 'b#2'), null);
	assert.equal(pane.visible, 'c#3');
});
