// Element classes by class ID: the ID of an element class is the name its code
// is registered under here (devdoc 2026102601 §3.5.1).
import type { Component } from 'svelte';
import type { ResolvedElement } from '../entries';
import ButtonElement from './ButtonElement.svelte';
import InputElement from './InputElement.svelte';
import ListElement from './ListElement.svelte';
import MenuElement from './MenuElement.svelte';
import SelectElement from './SelectElement.svelte';
import TextElement from './TextElement.svelte';

export type ElementProps = { panelId: string; element: ResolvedElement };

export const elementClasses: Record<string, Component<ElementProps>> = {
	text: TextElement,
	input: InputElement,
	select: SelectElement,
	button: ButtonElement,
	list: ListElement,
	menu: MenuElement
};
