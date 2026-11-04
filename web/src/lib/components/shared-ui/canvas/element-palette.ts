// The palette entries of element classes: value-editor classes in the VALUE EDITORS
// folder, all others in ELEMENTS (OpenSpec wpi-value-editor-elements, D7). Their IDs
// carry a prefix, so they are unique; dropping one adds a wpi-element node of that class.
// Validator classes go to VALIDATORS, in a sub-folder per category (OpenSpec
// wpi-primitive-validators, D9); dropping one onto an element adds it there.
import type { ElementClassSummary } from '$lib/wpi/api';
import { categories, type ValidatorClass } from '$lib/wpi/validators';
import type { NodeType } from '$lib/types/flow';

export const ELEMENT_CLASS_PREFIX = 'wpi-element:';
export const VALIDATOR_CLASS_PREFIX = 'wpi-validator:';

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

// Validator classes in the order of their categories (requirements §3), then by ID.
export function validatorPaletteTypes(classes: ValidatorClass[], title: string): NodeType[] {
	const rank = (c: ValidatorClass) => (categories.indexOf(c.category) + categories.length + 1) % (categories.length + 1);
	return [...classes]
		.sort((a, b) => rank(a) - rank(b) || a.descriptor_id.localeCompare(b.descriptor_id))
		.map((c) => ({
			id: VALIDATOR_CLASS_PREFIX + c.descriptor_id,
			label: c.descriptor_id,
			category: 'Validators',
			subcategory: c.category,
			title,
			icon: 'ShieldCheck',
			inputs: [],
			outputs: [],
			defaultData: {}
		}));
}
