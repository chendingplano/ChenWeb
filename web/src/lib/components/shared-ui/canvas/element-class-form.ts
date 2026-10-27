// The Create Element form's logic: an element class draft (format 0.3) built
// from the attributes the author lists (OpenSpec wpi-create-element-class,
// design D7, D9). The server's rules are the ones that count; these checks give
// quick feedback.
import type { Json, JsonObject } from '$lib/wpi/types';

export const CLASS_ID_PATTERN = /^[a-z0-9][a-z0-9-]{0,127}$/;
export const ATTRIBUTE_NAME_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/;
// Every class row may have these (devdoc 2026102601 §3.5.1); the form lists them from the start.
const CLASS_ATTRIBUTES = ['descriptor_id', 'schema_version', 'description', 'implement_notes'];
// Values that are always text, never read as JSON.
const TEXT_ONLY = ['descriptor_id', 'schema_version', 'implement_notes'];
// The key with which an instance names its class.
const RESERVED_NAMES = [...CLASS_ATTRIBUTES, 'element'];

// One attribute of the class row. A class attribute's value is the value itself;
// an instance attribute's value is an object with an optional help_tip and an
// optional default_value, null meaning every instance must set it.
export type DraftAttribute = { name: string; value: Json };
export type EditField = 'name' | 'value' | 'help_tip';

export type AttributeProblem = 'name' | 'reserved' | 'duplicate' | 'help_tip_en';
export type RowProblem = 'class_id' | 'description';

export function initialAttributes(): DraftAttribute[] {
	return [
		{ name: 'descriptor_id', value: '' },
		{ name: 'schema_version', value: '0.3' },
		{ name: 'description', value: { en: '', 'zh-cn': '' } },
		{ name: 'implement_notes', value: '' }
	];
}

// The form's attributes for a stored class row, to edit it: the class
// attributes first, in the order the create form lists them, then the instance
// attributes in the row's order. A description by language gets both language
// slots, as in the create form.
export function attributesFromRow(row: JsonObject): DraftAttribute[] {
	const classAttrs = initialAttributes().map((a) => {
		if (!(a.name in row)) return a;
		const v = row[a.name];
		if (a.name === 'description' && v !== null && typeof v === 'object' && !Array.isArray(v)) {
			return { name: a.name, value: { en: '', 'zh-cn': '', ...v } };
		}
		return { name: a.name, value: v };
	});
	const instanceAttrs = Object.entries(row)
		.filter(([name]) => !isClassAttribute(name))
		.map(([name, value]) => ({ name, value }));
	return [...classAttrs, ...instanceAttrs];
}

export function isClassAttribute(name: string): boolean {
	return CLASS_ATTRIBUTES.includes(name);
}

// An instance attribute's object, or an empty one.
function attrObject(attr: DraftAttribute): JsonObject {
	const v = attr.value;
	return v !== null && typeof v === 'object' && !Array.isArray(v) ? v : {};
}

export function hasDefault(attr: DraftAttribute): boolean {
	return 'default_value' in attrObject(attr);
}

export function defaultValue(attr: DraftAttribute): Json | undefined {
	return attrObject(attr).default_value;
}

export function helpTip(attr: DraftAttribute): Json | undefined {
	return attrObject(attr).help_tip;
}

// help_tip first, then default_value, each only when there is one.
function withParts(tip: Json | undefined, dv: Json | undefined, keepDefault: boolean): JsonObject {
	const out: JsonObject = {};
	if (tip !== undefined) out.help_tip = tip;
	if (keepDefault) out.default_value = dv ?? null;
	return out;
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

// The help tip from its English and Chinese texts, the empty ones left out.
function tipFrom(en: string, zh: string): Json | undefined {
	const tip: JsonObject = {};
	if (en.trim() !== '') tip.en = en.trim();
	if (zh.trim() !== '') tip['zh-cn'] = zh.trim();
	return Object.keys(tip).length > 0 ? tip : undefined;
}

function nameProblem(name: string, others: DraftAttribute[]): AttributeProblem | null {
	if (!ATTRIBUTE_NAME_PATTERN.test(name)) return 'name';
	if (RESERVED_NAMES.includes(name)) return 'reserved';
	if (others.some((a) => a.name === name)) return 'duplicate';
	return null;
}

export function attributeProblem(name: string, tipEn: string, tipZh: string, added: DraftAttribute[]): AttributeProblem | null {
	return nameProblem(name, added) ?? (tipZh.trim() !== '' && tipEn.trim() === '' ? 'help_tip_en' : null);
}

// A new instance attribute: required gives default_value null; an empty default
// leaves it out (optional, no default).
export function newAttribute(name: string, required: boolean, defaultText: string, tipEn: string, tipZh: string): DraftAttribute {
	const empty = defaultText.trim() === '';
	return { name, value: withParts(tipFrom(tipEn, tipZh), required ? null : empty ? undefined : parseDefault(defaultText), required || !empty) };
}

// Renaming an instance attribute; class attribute names cannot change.
export function renameProblem(oldName: string, newName: string, attributes: DraftAttribute[]): AttributeProblem | null {
	if (newName === oldName) return null;
	return nameProblem(newName, attributes.filter((a) => a.name !== oldName));
}

// The text an edit of a field starts from; applyEdit reads it back as the same value.
export function editText(attr: DraftAttribute, field: EditField): string {
	if (field === 'name') return attr.name;
	if (isClassAttribute(attr.name)) return TEXT_ONLY.includes(attr.name) ? String(attr.value) : JSON.stringify(attr.value);
	if (field === 'value' && !hasDefault(attr)) return '';
	const part = field === 'help_tip' ? helpTip(attr) : defaultValue(attr);
	return part === undefined ? '' : JSON.stringify(part);
}

// The attribute after an edit of its value or help tip. For an instance
// attribute, empty text removes the default (optional, no default) or the help tip.
export function applyEdit(attr: DraftAttribute, field: 'value' | 'help_tip', text: string): DraftAttribute {
	if (isClassAttribute(attr.name)) {
		return { name: attr.name, value: TEXT_ONLY.includes(attr.name) ? text.trim() : parseDefault(text) };
	}
	const empty = text.trim() === '';
	if (field === 'help_tip') {
		return { name: attr.name, value: withParts(empty ? undefined : parseDefault(text), defaultValue(attr), hasDefault(attr)) };
	}
	return { name: attr.name, value: withParts(helpTip(attr), empty ? undefined : parseDefault(text), !empty) };
}

// Texts by language with the empty ones left out, so that an unfilled
// Chinese description is not sent.
function withoutEmptyTexts(value: Json): Json {
	if (value === null || typeof value !== 'object' || Array.isArray(value)) return value;
	return Object.fromEntries(Object.entries(value).filter(([, t]) => t !== ''));
}

// The class row the create and update endpoints store. Empty implement_notes
// and an empty description are left out (an active class may have none).
export function buildClassRow(attributes: DraftAttribute[]): JsonObject {
	const row: JsonObject = {};
	for (const a of attributes) {
		if (a.name === 'implement_notes' && a.value === '') continue;
		const value = a.name === 'description' ? withoutEmptyTexts(a.value) : a.value;
		if (a.name === 'description' && (value === '' || (typeof value === 'object' && value !== null && Object.keys(value).length === 0))) continue;
		row[a.name] = value;
	}
	return row;
}

export function rowProblem(row: JsonObject): RowProblem | null {
	if (typeof row.descriptor_id !== 'string' || !CLASS_ID_PATTERN.test(row.descriptor_id)) return 'class_id';
	const d = row.description;
	const english = typeof d === 'string' ? d : d && typeof d === 'object' && !Array.isArray(d) ? d.en : undefined;
	if (typeof english !== 'string' || english.trim() === '') return 'description';
	return null;
}
