import { describe, expect, test } from 'bun:test';
import { PENDING_EDITORS, VALUE_EDITOR_IDS } from '$lib/components/shared-ui/canvas/class-editor';
import { valueEditors } from '$lib/components/shared-ui/property-editor/editors';
import { drawingEditor } from './editor-ids';

describe('drawingEditor', () => {
	test('every catalogue ID with code resolves to a registered editor', () => {
		for (const id of VALUE_EDITOR_IDS) {
			const editor = drawingEditor(id);
			if (PENDING_EDITORS.has(id)) expect(editor).toBeUndefined();
			else expect(valueEditors[editor!]).toBeDefined();
		}
	});

	test('variants use their base editor', () => {
		expect(drawingEditor('font-picker')).toBe('text');
		expect(drawingEditor('alignment-picker')).toBe('value-picker');
		expect(drawingEditor('action-picker')).toBe('value-picker');
		expect(drawingEditor('layouter-picker')).toBe('value-picker');
	});

	test('the icon picker and the data-source controls have their own editors', () => {
		for (const id of ['icon-picker', 'table-picker', 'field-picker', 'sort-order-editor']) expect(drawingEditor(id)).toBe(id);
	});

	test('unknown IDs have no editor', () => {
		expect(drawingEditor('slider')).toBeUndefined();
	});

	test('emoji-picker resolves to its own editor', () => {
		expect(drawingEditor('emoji-picker')).toBe('emoji-picker');
		expect(valueEditors['emoji-picker']).toBeDefined();
	});
});
