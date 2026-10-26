// Element types by name (devdoc 2026102601 §3.5, design D10).
import type { Component } from 'svelte';
import type { Origin, ResolvedElement } from '../entries';
import ButtonElement from './ButtonElement.svelte';
import InputElement from './InputElement.svelte';
import ListElement from './ListElement.svelte';
import MenuElement from './MenuElement.svelte';
import SelectElement from './SelectElement.svelte';
import TextElement from './TextElement.svelte';

export type ElementProps = { panelId: string; element: ResolvedElement };

export const elementTypes: Record<string, Component<ElementProps>> = {
	text: TextElement,
	input: InputElement,
	select: SelectElement,
	button: ButtonElement,
	list: ListElement,
	menu: MenuElement
};

// Where a trigger stored under attr (plus a deeper path) lives, for the server.
export function triggerOrigin(element: ResolvedElement, attr: string, rest = ''): Origin & { pointer: string } {
	const origin = element.origin(attr);
	return { ...origin, pointer: `${origin.base}/${attr}${rest}` };
}
