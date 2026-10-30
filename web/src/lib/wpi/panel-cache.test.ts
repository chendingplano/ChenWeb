import test from 'node:test';
import assert from 'node:assert/strict';
import { findCached, forget, paneInstanceId, paneOwner, sharingKey, show, type PaneModel } from './panel-cache';

test('page instances of the same page with the same input share a key', () => {
	assert.equal(sharingKey('customers', {}), sharingKey('customers', undefined));
	assert.equal(sharingKey('f', { doc: { b: 1, a: 2 } }), sharingKey('f', { doc: { a: 2, b: 1 } }));
	assert.notEqual(sharingKey('f', { ref: { kind: 'page', id: 'l', pointer: '/x' }, args: { id: 7 } }), sharingKey('f', { ref: { kind: 'page', id: 'l', pointer: '/x' }, args: { id: 9 } }));
});

test('switching hides, the cache shows the same instance again, closing forgets', () => {
	const pane: PaneModel = { opened: [], visible: null };
	const keys = { 'form#1': { key: 'form|7' }, 'form#2': { key: 'form|9' } };
	show(pane, 'form#1');
	show(pane, 'form#2');
	assert.deepEqual(pane, { opened: ['form#1', 'form#2'], visible: 'form#2' }, 'form#1 is hidden, not closed');
	assert.equal(findCached(pane, keys, 'form|7'), 'form#1');
	assert.equal(findCached(pane, keys, 'form|3'), null);
	show(pane, 'form#1');
	assert.deepEqual(pane.opened, ['form#2', 'form#1'], 'most recently shown last');
	forget(pane, 'form#1');
	assert.deepEqual(pane, { opened: ['form#2'], visible: null }, 'the pane shows its own panel again');
	forget(pane, 'form#2');
	assert.deepEqual(pane, { opened: [], visible: null });
});

test('pane instance IDs name their page instance', () => {
	const id = paneInstanceId('customer-admin-home#1', 'middle');
	assert.equal(id, 'customer-admin-home#1/middle');
	assert.equal(paneOwner(id), 'customer-admin-home#1');
});
