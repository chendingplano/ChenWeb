// The value editor for a property no definition describes (OpenSpec
// wpi-property-editors, design D3): chosen from its name and default value.
import type { Json } from '../types';
import type { ValueEditorId } from './types';

const textNames = new Set(['label', 'text', 'description', 'placeholder', 'help_text', 'help_tip']);

function isLanguageMap(value: Json | undefined): boolean {
	return !!value && typeof value === 'object' && !Array.isArray(value) && typeof value.en === 'string';
}

export function isLengthName(name: string): boolean {
	return /(^|_)(width|height)$/.test(name) || /_(gap|margin|size|thickness)$/.test(name);
}

export function commonSenseEditor(name: string, defaultValue: Json | undefined, fixed = false): ValueEditorId {
	if (fixed) return 'read-only';
	if (typeof defaultValue === 'boolean') return 'switch';
	if (typeof defaultValue === 'number') return 'number';
	if (isLengthName(name)) return 'length';
	if (name.endsWith('color')) return 'color';
	if (textNames.has(name) || isLanguageMap(defaultValue)) return 'localized-text';
	if (defaultValue !== null && typeof defaultValue === 'object') return 'json';
	return 'text';
}
