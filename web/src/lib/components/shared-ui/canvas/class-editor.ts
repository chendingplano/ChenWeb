// Logical attributes of stored classes. Element default_value wrappers are a
// storage detail: paths and editor metadata refer to the values authors see.
import type { Json, JsonObject, Text } from '$lib/wpi/types';

export type ClassKind = 'element' | 'action' | 'page-layouter' | 'element-layouter';
export type ClassTarget = { kind: ClassKind; id: string };
export type AttributeDefinition = {
	description?: Text;
	scope?: 'class' | 'instance';
	value_editor?: { id: string; options?: JsonObject };
};
export type AttributeNode = {
	name: string;
	path: string;
	value: Json | undefined;
	children: AttributeNode[];
};
export const CLASS_KEYS = new Set([
	'descriptor_id',
	'schema_version',
	'code_module_id',
	'description',
	'implement_notes',
	'attribute_definitions'
]);
export const VALUE_EDITOR_IDS = [
	'text',
	'multiline-text',
	'localized-text',
	'number',
	'switch',
	'value-picker',
	'ordered-multi-picker',
	'string-list',
	'date-picker',
	'time-picker',
	'color',
	'length',
	'font-picker',
	'alignment-picker',
	'icon-picker',
	'button-type-picker',
	'database-picker',
	'schema-picker',
	'table-picker',
	'field-picker',
	'data-source',
	'column-list',
	'sort-order-editor',
	'validation',
	'action-picker',
	'layouter-picker',
	'read-only',
	'json'
] as const;
export const PENDING_EDITORS = new Set([
	'date-picker',
	'time-picker',
	'button-type-picker',
	'database-picker',
	'schema-picker'
]);

export function object(value: unknown): JsonObject {
	return value !== null && typeof value === 'object' && !Array.isArray(value)
		? (value as JsonObject)
		: {};
}

export function classTarget(
	node: { type?: string; data: Record<string, unknown> } | null,
	page: JsonObject | null,
	mode: string | null,
	pane: string | null
): ClassTarget | null {
	if (node) {
		const kind =
			node.type === 'wpi-element' ? 'element' : node.type === 'wpi-action' ? 'action' : null;
		const id = kind ? (node.data[kind] ?? object(node.data.class).descriptor_id) : null;
		return kind && typeof id === 'string' && id ? { kind, id } : null;
	}
	if (!page) return null;
	if (mode === 'panes' && pane) {
		const id = object(page.page_layouter).layouter;
		return typeof id === 'string' && id ? { kind: 'page-layouter', id } : null;
	}
	if (mode === 'panel' && pane) {
		const panel = object(object(page.panels)[pane]);
		const id = object(panel.elem_layouter).layouter;
		return typeof id === 'string' && id ? { kind: 'element-layouter', id } : null;
	}
	return null;
}

const escape = (name: string) => name.replace(/~/g, '~0').replace(/\//g, '~1');
const segments = (path: string) =>
	path
		.slice(1)
		.split('/')
		.map((s) => s.replace(/~1/g, '/').replace(/~0/g, '~'));
// Class rows contain JSON only; this also accepts Svelte's reactive proxies.
const clone = (row: JsonObject): JsonObject => JSON.parse(JSON.stringify(row));

function logicalRow(kind: ClassKind, row: JsonObject): JsonObject {
	return Object.fromEntries(
		Object.entries(row)
			.filter(([name]) => name !== 'attribute_definitions')
			.map(([name, value]) => [
				name,
				kind === 'element' && !CLASS_KEYS.has(name) ? object(value).default_value : value
			])
	) as JsonObject;
}

export function attributeTree(kind: ClassKind, row: JsonObject): AttributeNode[] {
	function nodes(value: JsonObject | Json[], parent = ''): AttributeNode[] {
		return Object.entries(value).map(([name, value]) => {
			const path = `${parent}/${escape(name)}`;
			return {
				name,
				path,
				value,
				children: value !== null && typeof value === 'object' ? nodes(value, path) : []
			};
		});
	}
	return nodes(logicalRow(kind, row));
}

// All edits clone the row; no class copy or live instance is mutated.
export function setAttributeDefault(
	kind: ClassKind,
	row: JsonObject,
	path: string,
	value: Json | undefined
): JsonObject {
	const next = clone(row);
	const [head, ...rest] = segments(path);
	let container = next;
	let key = head;
	if (kind === 'element' && !CLASS_KEYS.has(head)) {
		container = next[head] = object(next[head]);
		key = 'default_value';
	}
	for (const segment of rest) {
		const child = container[key];
		if (child === null || typeof child !== 'object') throw new Error('Invalid attribute path');
		container = child as JsonObject;
		key = segment;
	}
	if (value === undefined) delete container[key];
	else container[key] = value;
	return next;
}

export function definitionAt(kind: ClassKind, row: JsonObject, path: string): AttributeDefinition {
	const [head, ...rest] = segments(path);
	const attr = object(row[head]);
	if (kind === 'element' && !CLASS_KEYS.has(head) && rest.length === 0) {
		return {
			...(attr.help_tip !== undefined ? { description: attr.help_tip as Text } : {}),
			...(attr.scope !== undefined ? { scope: attr.scope as AttributeDefinition['scope'] } : {}),
			...(attr.value_editor !== undefined
				? { value_editor: attr.value_editor as AttributeDefinition['value_editor'] }
				: {})
		};
	}
	return object(object(row.attribute_definitions)[path]) as AttributeDefinition;
}

function cleanDescription(value: Text | undefined): Text | undefined {
	if (typeof value === 'string') return value.trim() ? value : undefined;
	if (!value) return undefined;
	const texts = Object.fromEntries(
		Object.entries(value).filter(
			([language, text]) => typeof text === 'string' && (language === 'en' || text.trim())
		)
	);
	return Object.keys(texts).length ? texts : undefined;
}

export function setAttributeDefinition(
	kind: ClassKind,
	row: JsonObject,
	path: string,
	patch: AttributeDefinition
): JsonObject {
	const next = clone(row);
	const [head, ...rest] = segments(path);
	const attr = object(next[head]);
	if (kind === 'element' && !CLASS_KEYS.has(head) && rest.length === 0) {
		if (patch.scope !== undefined) attr.scope = patch.scope;
		if (patch.value_editor !== undefined)
			attr.value_editor = clone(patch.value_editor as JsonObject);
		if (patch.scope === 'class') {
			if (attr.default_value === undefined || attr.default_value === null)
				throw new Error('fixed_default');
			attr.instance_modifiable = false;
		} else if (patch.scope === 'instance') delete attr.instance_modifiable;
		if ('description' in patch) {
			const description = cleanDescription(patch.description);
			if (description === undefined) delete attr.help_tip;
			else attr.help_tip = description as Json;
		}
		next[head] = attr;
		// Top-level element definitions live together in the attribute, not in a second map.
		if (next.attribute_definitions) delete object(next.attribute_definitions)[path];
		return next;
	}
	const definitions = object(next.attribute_definitions);
	const definition = { ...definitionAt(kind, next, path), ...patch } as JsonObject;
	const description = cleanDescription(definition.description as Text | undefined);
	if (description === undefined) delete definition.description;
	else definition.description = description as Json;
	definitions[path] = definition;
	next.attribute_definitions = definitions;
	return next;
}

export function renameAttribute(
	kind: ClassKind,
	row: JsonObject,
	path: string,
	name: string
): JsonObject {
	const parts = segments(path);
	const oldName = parts.pop()!;
	if (oldName === name) return row;
	if (
		!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name) ||
		['__proto__', 'prototype', 'constructor', 'attribute_definitions'].includes(name)
	)
		throw new Error('name');
	if (parts.length === 0 && (CLASS_KEYS.has(oldName) || CLASS_KEYS.has(name)))
		throw new Error('reserved');
	const next = clone(row);
	let parent: JsonObject = logicalRow(kind, next);
	for (const part of parts) parent = parent[part] as JsonObject;
	if (Array.isArray(parent) || Object.hasOwn(parent, name)) throw new Error('duplicate');
	const renamed = Object.fromEntries(
		Object.entries(parent).map(([key, value]) => [key === oldName ? name : key, value])
	);
	let result: JsonObject;
	if (!parts.length)
		result = Object.fromEntries(
			Object.entries(next).map(([key, value]) => [key === oldName ? name : key, value])
		);
	else result = setAttributeDefault(kind, next, '/' + parts.map(escape).join('/'), renamed);
	const newPath = '/' + [...parts, name].map(escape).join('/');
	if (next.attribute_definitions) {
		result.attribute_definitions = Object.fromEntries(
			Object.entries(object(next.attribute_definitions)).map(([key, value]) => [
				key === path || key.startsWith(path + '/') ? newPath + key.slice(path.length) : key,
				value
			])
		);
	}
	return result;
}
