import test from 'node:test';
import assert from 'node:assert/strict';
import { actionRef, isAction, mergeAction, mergeElement, mergeInstance, resolveEntry, staticInput, textIn } from './entries';
import type { ClassRow, ElementInstance, PageDescriptor } from './types';

const page = (descriptor_id: string, extra: Partial<PageDescriptor> = {}): PageDescriptor => ({
	descriptor_id,
	schema_version: '0.5',
	page_layouter: { layouter: 'single-pane' },
	...extra
});

// Class rows, as the copies instances hold.
const classes = {
		input: {
			descriptor_id: 'input',
			schema_version: '0.4',
			code_module_id: 'input',
			description: 'A field',
			implement_notes: 'A one-line field',
			label: { help_tip: 'Caption', default_value: null },
			data_binding: { default_value: null },
			binding_mode: { default_value: 'read-write' },
			input_type: { help_tip: 'text, number or email' },
			width: { default_value: 'elastic' }
		},
		text: { descriptor_id: 'text', schema_version: '0.4', code_module_id: 'text' },
		// Drawn by the input code module; fixes its binding mode and input type.
		'email-input': {
			descriptor_id: 'email-input',
			schema_version: '0.4',
			code_module_id: 'input',
			label: { default_value: null },
			data_binding: { default_value: null },
			binding_mode: { default_value: 'read-write', instance_modifiable: false },
			input_type: { default_value: 'email', instance_modifiable: false }
		}
} as unknown as Record<string, ClassRow>;

// An instance holding a copy of its class.
const inst = (element: string, own: Partial<ElementInstance> = {}): ElementInstance => ({ element, class: classes[element], ...own });

test('an instance takes its class defaults and overrides them', () => {
	const r = resolveEntry(page('customer-form'), 'main', inst('input', { label: 'Email', data_binding: '$input_doc.email', binding_mode: 'read-only' }), 2);
	assert.ok(!('error' in r));
	assert.equal(r.desc.element, 'input');
	assert.equal(r.desc.binding_mode, 'read-only');
	assert.equal(r.desc.width, 'elastic');
	assert.equal(r.desc.label, 'Email');
	assert.equal(r.pointer, '/panels/main/elem_list/2');
	assert.equal(r.module, 'input');
	assert.equal('description' in r.desc, false, 'class attributes are not merged');
	assert.equal('implement_notes' in r.desc, false, 'class attributes are not merged');
	assert.equal('code_module_id' in r.desc, false, 'class attributes are not merged');
	assert.equal('class' in r.desc, false, 'the copy is not merged');
	const bare = mergeElement(classes.input, {});
	assert.equal('data_binding' in bare, false, 'null means no default');
	assert.equal('input_type' in bare, false, 'no default_value means no default');
});

test('an action instance takes its class copy defaults, held flat', () => {
	const row = { descriptor_id: 'save', schema_version: '0.2', mode: 'upsert', key: 'id', fields: null };
	assert.deepEqual(mergeInstance(row, { table: 't' }), { mode: 'upsert', key: 'id', table: 't' });
	assert.deepEqual(mergeAction({ action: 'save', class: row, table: 't' }), { action: 'save', mode: 'upsert', key: 'id', table: 't' });
});

test('class editor metadata never becomes runtime action or element attributes', () => {
	const metadata = { '/mode': { value_editor: { id: 'value-picker', options: { choices: ['upsert'] } } } };
	assert.deepEqual(mergeInstance({ descriptor_id: 'save', schema_version: '0.2', mode: 'upsert', attribute_definitions: metadata }, {}), { mode: 'upsert' });
	assert.deepEqual(mergeElement({ descriptor_id: 'text', schema_version: '0.4', text: { default_value: 'Hello' }, attribute_definitions: metadata }, {}), { text: 'Hello' });
});

test('action instances are named by their place in the page', () => {
	const r = resolveEntry(page('customers'), 'left', inst('input'), 1);
	assert.ok(!('error' in r));
	assert.deepEqual(actionRef(r, 'items', '/0/on_click'), { kind: 'page', id: 'customers', pointer: '/panels/left/elem_list/1/items/0/on_click' });
});

test('an instance without a class copy is an error, unless the editor supplies its class', () => {
	assert.deepEqual(resolveEntry(page('p'), 'main', { element: 'nope' }, 0), { error: 'element "nope" has no class copy' });
	const r = resolveEntry(page('p'), 'main', { element: 'input', label: 'x' }, 0, classes);
	assert.ok(!('error' in r));
	assert.equal(r.desc.width, 'elastic');
});

test('binding mode: instance, then page, then class', () => {
	const viewer = page('viewer', { binding_mode: 'read-only' });
	const mode = (p: PageDescriptor, entry: { element: string; binding_mode?: 'read-only' | 'read-write' }) => {
		const r = resolveEntry(p, 'main', inst(entry.element, entry.binding_mode ? { binding_mode: entry.binding_mode } : {}), 0);
		assert.ok(!('error' in r));
		return r.desc.binding_mode;
	};
	assert.equal(mode(viewer, { element: 'input' }), 'read-only');
	assert.equal(mode(viewer, { element: 'input', binding_mode: 'read-write' }), 'read-write');
	assert.equal(mode(page('editor'), { element: 'input' }), 'read-write');
	assert.equal(mode(viewer, { element: 'text' }), undefined, 'only classes with a binding mode take the page\'s');
	assert.equal(mode(viewer, { element: 'email-input' }), 'read-write', 'a fixed binding mode is not the page\'s to change');
});

test('a class drawn by a shared code module, with fixed values', () => {
	const r = resolveEntry(page('p'), 'main', inst('email-input', { label: 'Email', data_binding: '$input_doc.email' }), 0);
	assert.ok(!('error' in r));
	assert.equal(r.module, 'input');
	assert.equal(r.desc.element, 'email-input');
	assert.equal(r.desc.input_type, 'email');
	// The server refuses an instance that sets a fixed attribute; the merge lets the class win anyway.
	assert.equal(mergeElement(classes['email-input'], { input_type: 'number' }).input_type, 'email');
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
