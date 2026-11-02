import { test, expect } from 'bun:test';
import { withClassEditors } from './class-definition';
import { elementEditor } from './index';

test('stored editor choices override module defaults without changing the module definition', () => {
	const original = elementEditor('input');
	const result = withClassEditors(original, {
		'/max_length': { value_editor: { id: 'number', options: { min: 2, max: 30, integer: true } } }
	});
	const prop = result.groups.flatMap((g) => g.properties).find((p) => p.name === 'max_length')!;
	expect(prop.editor).toBe('number');
	expect(
		typeof prop.options === 'function'
			? prop.options({ get: () => undefined, triggerActions: [], pageLayouters: [] })
			: prop.options
	).toMatchObject({ min: 2, max: 30, integer: true });
	expect(
		original.groups.flatMap((g) => g.properties).find((p) => p.name === 'max_length')!.options
	).not.toEqual(prop.options);
});

test('a changed editor reads and writes raw descriptor values; an equivalent action picker keeps its conversion', () => {
	const original = elementEditor('button');
	const raw = withClassEditors(original, {
		'/on_click': { value_editor: { id: 'json', options: {} } }
	})
		.groups.flatMap((g) => g.properties)
		.find((p) => p.name === 'on_click')!;
	expect(raw.read).toBeUndefined();
	expect(raw.write).toBeUndefined();
	const picker = withClassEditors(original, {
		'/on_click': { value_editor: { id: 'action-picker', options: {} } }
	})
		.groups.flatMap((g) => g.properties)
		.find((p) => p.name === 'on_click')!;
	expect(picker.read!({ action: 'save' })).toBe('save');
});

test('class-defined unknown attributes appear with their chosen editor; pending editors fall back to JSON', () => {
	const def = withClassEditors(elementEditor('text'), {
		'/custom_date': { value_editor: { id: 'date-picker', options: { range: true } } },
		'/custom_count': { value_editor: { id: 'number', options: {} } }
	});
	const props = def.groups.flatMap((g) => g.properties);
	expect(props.find((p) => p.name === 'custom_date')!.editor).toBe('json');
	expect(props.find((p) => p.name === 'custom_count')!.editor).toBe('number');
});
