// The Create Element form's logic: an element class draft built from what the
// author enters (OpenSpec wpi-create-element-class, design D7). The server's
// rules are the ones that count; these checks give quick feedback.
import type { Json, JsonObject } from '$lib/wpi/types';

export const CLASS_ID_PATTERN = /^[a-z0-9][a-z0-9-]{0,127}$/;
const ATTRIBUTE_NAME_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/;
// Class attributes, and the key with which an instance names its class.
const RESERVED_NAMES = ['descriptor_id', 'schema_version', 'description', 'element'];

// One instance attribute of the class: its default, or null when every
// instance must set it.
export type DraftAttribute = { name: string; value: Json };

export type AttributeProblem = 'name' | 'reserved' | 'duplicate' | 'no_default';

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

export function attributeProblem(
	name: string,
	required: boolean,
	defaultText: string,
	added: DraftAttribute[]
): AttributeProblem | null {
	if (!ATTRIBUTE_NAME_PATTERN.test(name)) return 'name';
	if (RESERVED_NAMES.includes(name)) return 'reserved';
	if (added.some((a) => a.name === name)) return 'duplicate';
	if (!required && defaultText.trim() === '') return 'no_default';
	return null;
}

export function draftAttribute(name: string, required: boolean, defaultText: string): DraftAttribute {
	return { name, value: required ? null : parseDefault(defaultText) };
}

// The class row the create endpoint stores as a draft.
export function buildClassRow(id: string, descriptionEn: string, descriptionZh: string, attributes: DraftAttribute[]): JsonObject {
	const description: JsonObject = { en: descriptionEn.trim() };
	if (descriptionZh.trim() !== '') description['zh-cn'] = descriptionZh.trim();
	const row: JsonObject = { descriptor_id: id, schema_version: '0.2', description };
	for (const a of attributes) row[a.name] = a.value;
	return row;
}
