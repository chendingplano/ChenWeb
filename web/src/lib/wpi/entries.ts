// Merges instances with their class rows (devdoc 2026102601 §3.5.1): the row's
// non-null defaults overlaid with the instance's own attributes. Same rule as
// mergeInstance in server/api/wpi.
import type { ActionInstance, ActionRef, Bundle, ClassRow, ElementInstance, Input, InputSpec, JsonObject, PageDescriptor, Text } from './types';

const CLASS_ATTRS = new Set(['descriptor_id', 'schema_version', 'description', 'implement_notes', 'code_module_id']);

// An action class row holds each instance attribute's default itself.
export function mergeInstance<T extends object>(row: ClassRow | undefined, own: T): T {
	const defaults: Record<string, unknown> = {};
	for (const [k, v] of Object.entries(row ?? {})) {
		if (v !== null && !CLASS_ATTRS.has(k)) defaults[k] = v;
	}
	return { ...defaults, ...own } as T;
}

// An element class row (format 0.4) holds each instance attribute as
// {help_tip, default_value, instance_modifiable}; null or no default_value is no
// default.
// An attribute with instance_modifiable false is fixed by the class: its
// default_value is every instance's value, applied last (OpenSpec wpi-code-modules).
export function mergeElement<T extends object>(row: ClassRow | undefined, own: T): T {
	const defaults: Record<string, unknown> = {};
	const fixed: Record<string, unknown> = {};
	for (const [k, v] of Object.entries(row ?? {})) {
		if (CLASS_ATTRS.has(k) || v === null || typeof v !== 'object') continue;
		const attr = v as { default_value?: unknown; instance_modifiable?: unknown };
		if (attr.default_value === undefined || attr.default_value === null) continue;
		(attr.instance_modifiable === false ? fixed : defaults)[k] = attr.default_value;
	}
	return { ...defaults, ...own, ...fixed } as T;
}

// Whether an element class row fixes an attribute for all its instances.
export function isFixed(row: ClassRow, attr: string): boolean {
	const v = row[attr];
	return v !== null && typeof v === 'object' && (v as { instance_modifiable?: unknown }).instance_modifiable === false;
}

export type ResolvedElement = {
	// The instance merged with its class.
	desc: ElementInstance;
	// The code module that draws it: its class's code_module_id.
	module: string;
	pageId: string;
	// JSON Pointer of the instance in its page. It identifies the instance, and
	// action instances inside it are named to the server below it.
	pointer: string;
};

// An element instance that does not set binding_mode takes its page's, when its
// class names one and does not fix it, so the order is instance, page, class
// (devdoc §3.2.1). Same rule as withPageDefaults in server/api/wpi.
export function resolveEntry(bundle: Bundle, page: PageDescriptor, entry: ElementInstance, index: number): ResolvedElement | { error: string } {
	const row = bundle.elements[entry.element];
	if (!row) return { error: `element class "${entry.element}" is not in the bundle` };
	const own =
		page.binding_mode && 'binding_mode' in row && !isFixed(row, 'binding_mode') && entry.binding_mode === undefined
			? { ...entry, binding_mode: page.binding_mode }
			: entry;
	const module = typeof row.code_module_id === 'string' ? row.code_module_id : '';
	return { desc: mergeElement(row, own), module, pageId: page.descriptor_id, pointer: `/elements/${index}` };
}

// Names the action instance stored under attr (plus a deeper path) of an element.
export function actionRef(element: ResolvedElement, attr: string, rest = ''): ActionRef {
	return { kind: 'page', id: element.pageId, pointer: `${element.pointer}/${attr}${rest}` };
}

// The text for locale, else English (design D9).
export function textIn(text: Text | undefined, locale: string): string {
	if (text === undefined) return '';
	if (typeof text === 'string') return text;
	return text[locale] ?? text.en ?? '';
}

// An input with an action attribute is an action instance; any other object is
// static data, the input doc itself (devdoc §3.2.2).
export function isAction(input: Input): input is ActionInstance {
	return typeof input.action === 'string';
}

// Static data as a panel input; {} stays the plain empty input.
export function staticInput(doc: JsonObject): InputSpec {
	return Object.keys(doc).length ? { doc } : {};
}
