// The palette entries of element classes: value-editor classes in the VALUE EDITORS
// folder, all others in ELEMENTS (OpenSpec wpi-value-editor-elements, D7). Their IDs
// carry a prefix, so they are unique; dropping one adds a wpi-element node of that class.
import type { ElementClassSummary } from '$lib/wpi/api';
import type { NodeType } from '$lib/types/flow';

export const ELEMENT_CLASS_PREFIX = 'wpi-element:';

export function elementPaletteTypes(
	classes: ElementClassSummary[],
	draftLabel: (id: string) => string
): NodeType[] {
	return classes.map((c) => ({
		id: ELEMENT_CLASS_PREFIX + c.descriptor_id,
		label: c.status === 'draft' ? draftLabel(c.descriptor_id) : c.descriptor_id,
		category: c.code_module_id === 'value-editor' ? 'Value editors' : 'Elements',
		icon: 'square-mouse-pointer',
		inputs: [],
		outputs: [],
		defaultData: {}
	}));
}
