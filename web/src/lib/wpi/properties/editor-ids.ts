// Which value editor draws each ID of the catalogue in devdoc 2026102701 §1.3.9 (the
// IDs the class editor offers). The Properties panel's class overrides and the
// value-editor element both resolve IDs here, so the two cannot disagree (OpenSpec
// wpi-value-editor-elements, D4).
import type { ValueEditorId } from './types';

const drawn = new Set<string>([
	'text',
	'multiline-text',
	'localized-text',
	'number',
	'switch',
	'value-picker',
	'ordered-multi-picker',
	'string-list',
	'color',
	'length',
	'icon-picker',
	'emoji-picker',
	'table-picker',
	'field-picker',
	'data-source',
	'column-list',
	'sort-order-editor',
	'validation',
	'read-only',
	'json'
]);

// Variants drawn by a base editor with their own options.
const aliases: Record<string, ValueEditorId> = {
	'font-picker': 'text',
	'alignment-picker': 'value-picker',
	'action-picker': 'value-picker',
	'layouter-picker': 'value-picker'
};

// The editor that draws a catalogue ID; undefined for one that has no code yet.
export function drawingEditor(id: string): ValueEditorId | undefined {
	return aliases[id] ?? (drawn.has(id) ? (id as ValueEditorId) : undefined);
}
