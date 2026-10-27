// The Create Element form's logic: an element class draft built from the
// attributes the author lists (OpenSpec wpi-create-element-class, design D7).
// The server's rules are the ones that count; these checks give quick feedback.
import type { Json, JsonObject } from '$lib/wpi/types';

export const CLASS_ID_PATTERN = /^[a-z0-9][a-z0-9-]{0,127}$/;
const ATTRIBUTE_NAME_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/;
// Every class row has these (devdoc 2026102601 §3.5.1); the form lists them from the start.
const CLASS_ATTRIBUTES = ['descriptor_id', 'schema_version', 'description'];
// Values that are always text, never read as JSON.
const TEXT_ONLY = ['descriptor_id', 'schema_version'];
// The key with which an instance names its class.
const RESERVED_NAMES = [...CLASS_ATTRIBUTES, 'element'];

// One attribute of the class row. For an instance attribute, null means every
// instance must set it.
export type DraftAttribute = { name: string; value: Json };

export type AttributeProblem = 'name' | 'reserved' | 'duplicate' | 'no_default' | 'empty_value';
export type RowProblem = 'class_id' | 'description';

export function initialAttributes(): DraftAttribute[] {
	return [
		{ name: 'descriptor_id', value: '' },
		{ name: 'schema_version', value: '0.2' },
		{ name: 'description', value: { en: '', 'zh-cn': '' } }
	];
}

export function isClassAttribute(name: string): boolean {
	return CLASS_ATTRIBUTES.includes(name);
}

// A default as typed: JSON when it parses (200, true, {"en": "Name"}),
// otherwise the text itself (200px, read-write).
export function parseDefault(text: string): Json {
	const t = text.trim();
	try {
		return JSON.parse(t) as Json;
	} catch {
		return t;
	}
}

// The value an edit of the named attribute stores.
export function parseValue(name: string, text: string): Json {
	return TEXT_ONLY.includes(name) ? text.trim() : parseDefault(text);
}

// The text an edit of the named attribute starts from; parseValue reads it back
// as the same value.
export function editText(attr: DraftAttribute): string {
	return TEXT_ONLY.includes(attr.name) ? String(attr.value) : JSON.stringify(attr.value);
}

function nameProblem(name: string, others: DraftAttribute[]): AttributeProblem | null {
	if (!ATTRIBUTE_NAME_PATTERN.test(name)) return 'name';
	if (RESERVED_NAMES.includes(name)) return 'reserved';
	if (others.some((a) => a.name === name)) return 'duplicate';
	return null;
}

export function attributeProblem(
	name: string,
	required: boolean,
	defaultText: string,
	added: DraftAttribute[]
): AttributeProblem | null {
	return nameProblem(name, added) ?? (!required && defaultText.trim() === '' ? 'no_default' : null);
}

// Renaming an instance attribute; class attribute names cannot change.
export function renameProblem(oldName: string, newName: string, attributes: DraftAttribute[]): AttributeProblem | null {
	if (newName === oldName) return null;
	return nameProblem(newName, attributes.filter((a) => a.name !== oldName));
}

// An edit may leave a class attribute empty (create reports it), but not an
// instance attribute: its value is a default, or null.
export function editProblem(name: string, text: string): AttributeProblem | null {
	return !isClassAttribute(name) && text.trim() === '' ? 'empty_value' : null;
}

export function draftAttribute(name: string, required: boolean, defaultText: string): DraftAttribute {
	return { name, value: required ? null : parseDefault(defaultText) };
}

// Texts by language with the empty ones left out, so that an unfilled
// Chinese description is not sent.
function withoutEmptyTexts(value: Json): Json {
	if (value === null || typeof value !== 'object' || Array.isArray(value)) return value;
	return Object.fromEntries(Object.entries(value).filter(([, t]) => t !== ''));
}

// The class row the create endpoint stores as a draft.
export function buildClassRow(attributes: DraftAttribute[]): JsonObject {
	const row: JsonObject = {};
	for (const a of attributes) row[a.name] = a.name === 'description' ? withoutEmptyTexts(a.value) : a.value;
	return row;
}

export function rowProblem(row: JsonObject): RowProblem | null {
	if (typeof row.descriptor_id !== 'string' || !CLASS_ID_PATTERN.test(row.descriptor_id)) return 'class_id';
	const d = row.description;
	const english = typeof d === 'string' ? d : d && typeof d === 'object' && !Array.isArray(d) ? d.en : undefined;
	if (typeof english !== 'string' || english.trim() === '') return 'description';
	return null;
}
