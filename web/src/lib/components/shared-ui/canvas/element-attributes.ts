// The instance attributes of a WPI element node, as the editor's properties
// panel shows and edits them. Typed text is read the way the Create Element form
// reads a default: JSON when it parses, otherwise the text itself.
import type { Json, JsonObject } from '$lib/wpi/types';
import { parseDefault } from './element-class-form';

// The text a value is shown as; reading it back gives the same value, so a
// string that would read as something else ("200", "true") is shown quoted.
export function valueText(value: Json | undefined): string {
	if (value === undefined) return '';
	if (typeof value === 'string' && parseDefault(value) === value) return value;
	return JSON.stringify(value);
}

// The node's attributes after the author typed text for one of them. Empty text
// unsets it, so the class default applies.
export function withAttribute(attrs: JsonObject, name: string, text: string): JsonObject {
	const next = { ...attrs };
	if (text.trim() === '') delete next[name];
	else next[name] = parseDefault(text);
	return next;
}

// Replace only the nested field mappings, retaining all other source settings.
export function withSourceFields(attrs: JsonObject, fallback: Json | undefined, fields: Json[]): JsonObject {
	const source = attrs.source ?? fallback;
	if (!source || typeof source !== 'object' || Array.isArray(source)) return attrs;
	return { ...attrs, source: { ...source, fields } };
}

export function withSourceProperty(attrs: JsonObject, fallback: Json | undefined, key: string, value: Json | undefined): JsonObject {
	const current = attrs.source ?? fallback;
	const source: JsonObject = current && typeof current === 'object' && !Array.isArray(current) ? { ...current } : {};
	if (value === undefined) delete source[key];
	else source[key] = value;
	return { ...attrs, source };
}
