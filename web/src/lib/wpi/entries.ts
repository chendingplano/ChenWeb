// Merges instances with their class rows (devdoc 2026102601 §3.5.1): the row's
// non-null defaults overlaid with the instance's own attributes. Same rule as
// mergeInstance in server/api/wpi.
import type { ActionRef, Bundle, ClassRow, ElementInstance, Text } from './types';

const CLASS_ATTRS = new Set(['descriptor_id', 'schema_version', 'description']);

export function mergeInstance<T extends object>(row: ClassRow | undefined, own: T): T {
	const defaults: Record<string, unknown> = {};
	for (const [k, v] of Object.entries(row ?? {})) {
		if (v !== null && !CLASS_ATTRS.has(k)) defaults[k] = v;
	}
	return { ...defaults, ...own } as T;
}

export type ResolvedElement = {
	// The instance merged with its class.
	desc: ElementInstance;
	pageId: string;
	// JSON Pointer of the instance in its page. It identifies the instance, and
	// action instances inside it are named to the server below it.
	pointer: string;
};

export function resolveEntry(bundle: Bundle, pageId: string, entry: ElementInstance, index: number): ResolvedElement | { error: string } {
	const row = bundle.elements[entry.element];
	if (!row) return { error: `element class "${entry.element}" is not in the bundle` };
	return { desc: mergeInstance(row, entry), pageId, pointer: `/elements/${index}` };
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
