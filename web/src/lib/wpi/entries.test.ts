import test from 'node:test';
import assert from 'node:assert/strict';
import { actionRef, isAction, mergeElement, mergeInstance, resolveEntry, staticInput, textIn } from './entries';
import type { Bundle, PageDescriptor } from './types';

const page = (descriptor_id: string, extra: Partial<PageDescriptor> = {}): PageDescriptor => ({
	descriptor_id,
	schema_version: '0.2',
	layouter: 'flow-layouter',
	elements: [],
	...extra
});

const bundle = {
	elements: {
		input: {
			descriptor_id: 'input',
			schema_version: '0.3',
			description: 'A field',
			implement_notes: 'A one-line field',
			label: { help_tip: 'Caption', default_value: null },
			data_binding: { default_value: null },
			binding_mode: { default_value: 'read-write' },
			input_type: { help_tip: 'text, number or email' },
			width: { default_value: 'elastic' }
		},
		text: { descriptor_id: 'text', schema_version: '0.3' }
	}
} as unknown as Bundle;

test('an instance takes its class defaults and overrides them', () => {
	const r = resolveEntry(bundle, page('customer-form'), { element: 'input', label: 'Email', data_binding: '$input_doc.email', binding_mode: 'read-only' }, 2);
	assert.ok(!('error' in r));
	assert.equal(r.desc.element, 'input');
	assert.equal(r.desc.binding_mode, 'read-only');
	assert.equal(r.desc.width, 'elastic');
	assert.equal(r.desc.label, 'Email');
	assert.equal(r.pointer, '/elements/2');
	assert.equal('description' in r.desc, false, 'class attributes are not merged');
	assert.equal('implement_notes' in r.desc, false, 'class attributes are not merged');
	const bare = mergeElement(bundle.elements.input, {});
	assert.equal('data_binding' in bare, false, 'null means no default');
	assert.equal('input_type' in bare, false, 'no default_value means no default');
});

test('an action instance takes its class defaults, held flat', () => {
	const row = { descriptor_id: 'save', schema_version: '0.2', mode: 'upsert', key: 'id', fields: null };
	assert.deepEqual(mergeInstance(row, { table: 't' }), { mode: 'upsert', key: 'id', table: 't' });
});

test('action instances are named by their place in the page', () => {
	const r = resolveEntry(bundle, page('customers'), { element: 'input' }, 1);
	assert.ok(!('error' in r));
	assert.deepEqual(actionRef(r, 'items', '/0/on_click'), { kind: 'page', id: 'customers', pointer: '/elements/1/items/0/on_click' });
});

test('a class missing from the bundle is an error', () => {
	assert.deepEqual(resolveEntry(bundle, page('p'), { element: 'nope' }, 0), { error: 'element class "nope" is not in the bundle' });
});

test('binding mode: instance, then page, then class', () => {
	const viewer = page('viewer', { binding_mode: 'read-only' });
	const mode = (p: PageDescriptor, entry: { element: string; binding_mode?: 'read-only' | 'read-write' }) => {
		const r = resolveEntry(bundle, p, entry, 0);
		assert.ok(!('error' in r));
		return r.desc.binding_mode;
	};
	assert.equal(mode(viewer, { element: 'input' }), 'read-only');
	assert.equal(mode(viewer, { element: 'input', binding_mode: 'read-write' }), 'read-write');
	assert.equal(mode(page('editor'), { element: 'input' }), 'read-write');
	assert.equal(mode(viewer, { element: 'text' }), undefined, 'only classes with a binding mode take the page\'s');
});

test('static data versus action instances as input', () => {
	assert.equal(isAction({ action: 'load-file', file_id: 'about' } as never), true);
	assert.equal(isAction({ title: 'About' }), false);
	assert.deepEqual(staticInput({}), {});
	assert.deepEqual(staticInput({ title: 'About' }), { doc: { title: 'About' } });
});

test('text in the current language, else English', () => {
	assert.equal(textIn({ en: 'Phone', 'zh-cn': '电话' }, 'zh-cn'), '电话');
	assert.equal(textIn({ en: 'Phone' }, 'zh-cn'), 'Phone');
	assert.equal(textIn('Phone', 'zh-cn'), 'Phone');
	assert.equal(textIn(undefined, 'en'), '');
});
