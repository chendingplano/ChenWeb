// Property editor definitions by entity (OpenSpec wpi-property-editors, D1).
// Element definitions are keyed by code module ID, like the code that draws
// them (wpi/elements/index.ts); a module without one gets common-sense editors.
import { m } from '$lib/paraglide/messages.js';
import { textDefaults } from '../text-attributes';
import type { JsonObject } from '../types';
import { chatComposer, chatPanel } from './chat';
import { button, list, menu, select } from './elements';
import { input, inputDefaults } from './input';
import { text } from './text';
import { elementPlacement } from './shared';
import type { EditorDef } from './types';

export { page, pane, panel } from './page';

export const elementEditors: Record<string, EditorDef> = {
	text,
	input,
	select,
	button,
	list,
	menu,
	'chat-composer': chatComposer,
	'chat-panel': chatPanel
};

// The definition for a code module; an unknown module shows its placement only,
// and every class attribute in Other.
export function elementEditor(module: string): EditorDef {
	return { ...(elementEditors[module] ?? { groups: [elementPlacement] }), otherHelp: m.wpi_prop_other_element };
}

// Values a module uses when neither the instance nor its class sets one.
export function moduleFallbacks(module: string): JsonObject {
	return module === 'text' ? textDefaults : module === 'input' ? inputDefaults : {};
}
