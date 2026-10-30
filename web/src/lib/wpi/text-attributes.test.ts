import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { displayedText, textDefaults, validTextAttribute } from './text-attributes';
import { mergeElement, resolveEntry } from './entries';
import cases from '../../../../server/api/wpi/text-attribute-cases.json';
import type { ClassRow, ElementInstance, Json, PageDescriptor } from './types';

test('text client validation matches the server contract cases', () => {
	for (const c of cases)
		assert.equal(validTextAttribute(c.name, c.value as Json), c.valid, JSON.stringify(c));
});
test('text defaults match every concrete default in the reviewed descriptor', () => {
	const migration = readFileSync(
		new URL('../../../../project_migrations/20261030000004_wpi_text_r2_draft.sql', import.meta.url),
		'utf8'
	);
	const cls = JSON.parse(migration.split('$descriptor$')[1]);
	const defaults = Object.fromEntries(
		Object.entries(cls)
			.filter(([, v]) => v && typeof v === 'object' && 'default_value' in v)
			.map(([k, v]) => [k, (v as { default_value: unknown }).default_value])
	);
	assert.deepEqual(textDefaults, defaults);
});
test('literal locale maps and bound values have different conversion rules', () => {
	const d: ElementInstance = { element: 'text', text: { en: 'Hello', 'zh-cn': '你好' } };
	assert.equal(displayedText(d, undefined, 'zh-cn'), '你好');
	assert.equal(displayedText(d, undefined, 'fr'), 'Hello');
	for (const value of [undefined, null, 0, false, [1, 2], { en: 'Hello' }] as (
		| Json
		| undefined
	)[]) {
		assert.equal(
			displayedText({ ...d, data_binding: '$input_doc.value' }, value, 'zh-cn'),
			value == null ? '' : String(value)
		);
	}
});
test('old copies retain defaults and cannot acquire page binding mode', () => {
	const old: ClassRow = {
		descriptor_id: 'text',
		schema_version: '0.4',
		code_module_id: 'text',
		text: {},
		data_binding: {}
	};
	const page = { descriptor_id: 'p', binding_mode: 'read-write' } as PageDescriptor;
	const r = resolveEntry(page, 'main', { element: 'text', class: old, text: 'Old' }, 0);
	assert.ok(!('error' in r));
	assert.equal(r.desc.binding_mode, undefined);
	assert.equal(displayedText(r.desc, undefined, 'en'), 'Old');
	const fixed = { ...old, font_size: { default_value: '1rem', instance_modifiable: false } };
	assert.equal(mergeElement(fixed, { element: 'text', font_size: '2rem' }).font_size, '1rem');
	assert.equal(old.font_size, undefined);
});
