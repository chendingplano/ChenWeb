// Class metadata overrides the code module's default editor for new copies.
// Existing copies without metadata retain the code-defined property editor.
import { m } from '$lib/paraglide/messages.js';
import type { ValueEditorOptions } from '$lib/components/shared-ui/value-editors/types';
import type { JsonObject } from '../types';
import type { ElementClassSummary } from '../api';
import { drawingEditor } from './editor-ids';
import type { EditorDef, PropertyDef, ValueEditorId } from './types';

const reserved = new Set([
	'descriptor_id',
	'schema_version',
	'code_module_id',
	'description',
	'implement_notes',
	'attribute_definitions'
]);

export function elementClassDefinitions(
	cls: ElementClassSummary | null | undefined
): JsonObject | undefined {
	if (!cls) return undefined;
	const definitions = { ...cls.attribute_definitions };
	for (const [name, attribute] of Object.entries(cls.attributes)) {
		if (attribute.value_editor)
			definitions['/' + name.replace(/~/g, '~0').replace(/\//g, '~1')] = {
				value_editor: attribute.value_editor
			} as JsonObject;
	}
	return definitions;
}

export function withClassEditors(
	def: EditorDef,
	definitions: JsonObject | undefined,
	prefix = ''
): EditorDef {
	if (!definitions) return def;
	const configured = new Map<string, JsonObject>();
	for (const [path, value] of Object.entries(definitions)) {
		if (!path.startsWith(prefix + '/')) continue;
		const segment = path.slice(prefix.length + 1);
		if (!segment || segment.includes('/')) continue;
		const name = segment.replace(/~1/g, '/').replace(/~0/g, '~');
		if (reserved.has(name) || !value || typeof value !== 'object' || Array.isArray(value)) continue;
		const editor = value.value_editor;
		if (
			editor &&
			typeof editor === 'object' &&
			!Array.isArray(editor) &&
			typeof editor.id === 'string'
		)
			configured.set(name, editor);
	}
	if (!configured.size) return def;
	function apply(p: PropertyDef): PropertyDef {
		const setting = configured.get(p.name);
		if (!setting) return p;
		const id = String(setting.id);
		const editor: ValueEditorId = drawingEditor(id) ?? 'json';
		const options = (setting.options as ValueEditorOptions | undefined) ?? {};
		const base = p.options;
		return {
			...p,
			editor,
			...(p.editor === editor ? {} : { read: undefined, write: undefined }),
			options: (ctx) => ({
				...(p.editor === editor ? (typeof base === 'function' ? base(ctx) : base) : {}),
				...options
			})
		};
	}
	const known = new Set(def.groups.flatMap((g) => g.properties.map((p) => p.name)));
	const other = [...configured.keys()]
		.filter((name) => !known.has(name))
		.map((name) => apply({ name }));
	return {
		...def,
		groups: [
			...def.groups.map((g) => ({ ...g, properties: g.properties.map(apply) })),
			...(other.length
				? [{ id: 'class-editors', label: m.wpi_input_group_other, properties: other }]
				: [])
		]
	};
}
