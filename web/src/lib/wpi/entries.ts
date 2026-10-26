// Resolves a page's element entries: a stored element by reference (with the
// page's overrides applied) or an inline element. Each attribute remembers
// which stored descriptor it came from, so an inline action can be named to
// the server by its JSON Pointer (design D4).
import type { Bundle, ElementDescriptor, ElementEntry } from './types';

export type Origin = { kind: 'page' | 'element'; id: string; base: string };

export type ResolvedElement = {
	desc: ElementDescriptor;
	// Where an attribute's value is stored; base is the JSON Pointer of the
	// element object inside that descriptor.
	origin: (attr: string) => Origin;
};

export function resolveEntry(bundle: Bundle, pageId: string, entry: ElementEntry, index: number): ResolvedElement | { error: string } {
	const inPage: Origin = { kind: 'page', id: pageId, base: `/elements/${index}` };
	if ('element' in entry && entry.element) {
		const { element, ...overrides } = entry;
		const stored = bundle.elements[element];
		if (!stored) return { error: `element "${element}" is not in the bundle` };
		const inElement: Origin = { kind: 'element', id: element, base: '' };
		return {
			desc: { ...stored, ...overrides } as ElementDescriptor,
			origin: (attr) => (Object.hasOwn(overrides, attr) ? inPage : inElement)
		};
	}
	return { desc: entry as ElementDescriptor, origin: () => inPage };
}
