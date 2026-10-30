// Code modules by ID: the frontend code that draws elements. An element class
// names the module that draws it in code_module_id, and several classes may
// share one (devdoc 2026102601 §3.5.2; each module is documented in
// KnowledgeStore doc-repo/code_modules). The backend validator lists what each
// module accepts under the same IDs (codeModules in server/api/wpi/validate.go).
import type { Component } from 'svelte';
import type { ResolvedElement } from '../entries';
import ButtonElement from './ButtonElement.svelte';
import InputElement from './InputElement.svelte';
import ListElement from './ListElement.svelte';
import MenuElement from './MenuElement.svelte';
import SelectElement from './SelectElement.svelte';
import TextElement from './TextElement.svelte';

export type ElementProps = { instanceId: string; element: ResolvedElement };

export const codeModules: Record<string, Component<ElementProps>> = {
	text: TextElement,
	input: InputElement,
	select: SelectElement,
	button: ButtonElement,
	list: ListElement,
	menu: MenuElement
};
