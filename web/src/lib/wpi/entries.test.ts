import test from 'node:test';
import assert from 'node:assert/strict';
import { resolveEntry } from './entries';
import type { Bundle } from './types';

const bundle = {
	elements: {
		'customer-email': {
			descriptor_id: 'customer-email',
			element_type: 'input',
			label: 'Email',
			data_binding: '$input_doc.email',
			binding_mode: 'read-write',
			on_click: { action_type: 'close_page' }
		}
	}
} as unknown as Bundle;

test('a referenced element takes the page overrides and tracks where each attribute lives', () => {
	const r = resolveEntry(bundle, 'customer-form', { element: 'customer-email', binding_mode: 'read-only' }, 2);
	assert.ok(!('error' in r));
	assert.equal(r.desc.binding_mode, 'read-only');
	assert.equal(r.desc.label, 'Email');
	assert.equal('element' in r.desc, false);
	assert.deepEqual(r.origin('binding_mode'), { kind: 'page', id: 'customer-form', base: '/elements/2' });
	assert.deepEqual(r.origin('on_click'), { kind: 'element', id: 'customer-email', base: '' });
});

test('an inline element lives in the page', () => {
	const r = resolveEntry(bundle, 'p', { descriptor_id: 'save', element_type: 'button', label: 'Save' }, 0);
	assert.ok(!('error' in r));
	assert.deepEqual(r.origin('on_click'), { kind: 'page', id: 'p', base: '/elements/0' });
});

test('a reference missing from the bundle is an error', () => {
	assert.deepEqual(resolveEntry(bundle, 'p', { element: 'nope' }, 0), { error: 'element "nope" is not in the bundle' });
});
