import { expect, test } from 'bun:test';
import type { ElementClassSummary } from '$lib/wpi/api';
import { ELEMENT_CLASS_PREFIX, elementPaletteTypes } from './element-palette';

const cls = (descriptor_id: string, code_module_id: string, status: 'draft' | 'active' = 'active'): ElementClassSummary => ({
	descriptor_id,
	code_module_id,
	status,
	version: 1,
	description: null,
	attributes: {}
});

test('value-editor classes go to VALUE EDITORS, all other element classes to ELEMENTS', () => {
	const types = elementPaletteTypes(
		[cls('input', 'input'), cls('color-picker', 'value-editor'), cls('date-picker', 'value-editor', 'draft'), cls('slider', 'slider', 'draft')],
		(id) => `${id} (draft)`
	);
	expect(types.map((t) => [t.id, t.label, t.category])).toEqual([
		[ELEMENT_CLASS_PREFIX + 'input', 'input', 'Elements'],
		[ELEMENT_CLASS_PREFIX + 'color-picker', 'color-picker', 'Value editors'],
		[ELEMENT_CLASS_PREFIX + 'date-picker', 'date-picker (draft)', 'Value editors'],
		[ELEMENT_CLASS_PREFIX + 'slider', 'slider (draft)', 'Elements']
	]);
});
