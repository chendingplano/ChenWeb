import test from 'node:test';
import assert from 'node:assert/strict';
import { actionRef, mergeInstance, resolveEntry, textIn } from './entries';
import type { Bundle } from './types';

const bundle = {
	elements: {
		input: {
			descriptor_id: 'input',
			schema_version: '0.2',
			description: 'A field',
			label: null,
			data_binding: null,
			binding_mode: 'read-write',
			width: 'elastic'
		}
	}
} as unknown as Bundle;

test('an instance takes its class defaults and overrides them', () => {
	const r = resolveEntry(bundle, 'customer-form', { element: 'input', label: 'Email', data_binding: '$input_doc.email', binding_mode: 'read-only' }, 2);
	assert.ok(!('error' in r));
	assert.equal(r.desc.element, 'input');
	assert.equal(r.desc.binding_mode, 'read-only');
	assert.equal(r.desc.width, 'elastic');
	assert.equal(r.desc.label, 'Email');
	assert.equal(r.pointer, '/elements/2');
	assert.equal('description' in r.desc, false, 'class attributes are not merged');
	assert.equal('data_binding' in mergeInstance(bundle.elements.input, {}), false, 'null means no default');
});

test('action instances are named by their place in the page', () => {
	const r = resolveEntry(bundle, 'customers', { element: 'input' }, 1);
	assert.ok(!('error' in r));
	assert.deepEqual(actionRef(r, 'items', '/0/on_click'), { kind: 'page', id: 'customers', pointer: '/elements/1/items/0/on_click' });
});

test('a class missing from the bundle is an error', () => {
	assert.deepEqual(resolveEntry(bundle, 'p', { element: 'nope' }, 0), { error: 'element class "nope" is not in the bundle' });
});

test('text in the current language, else English', () => {
	assert.equal(textIn({ en: 'Phone', 'zh-cn': '电话' }, 'zh-cn'), '电话');
	assert.equal(textIn({ en: 'Phone' }, 'zh-cn'), 'Phone');
	assert.equal(textIn('Phone', 'zh-cn'), 'Phone');
	assert.equal(textIn(undefined, 'en'), '');
});
