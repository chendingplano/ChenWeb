// Validators as elements use them (OpenSpec wpi-primitive-validators, design D3,
// D8): an element's validators list holds validator instances, each with a whole
// copy of its class, its own constraint values and messages. All must pass (allOf).
import type { Json, JsonObject, Text } from '../types';
import { runValidator, type ValidationError } from './run';

export { categories, effectiveConstraints, modules, runValidator } from './run';
export type { ConstraintSpec, ModuleSpec, ValidationError, ValidationResult } from './run';

// The element code modules whose instances may set validators, as validatorModules
// in server/api/wpi/validator_classes.go (design D3).
export const validatorModules = ['input', 'select', 'value-editor'];
export const acceptsValidators = (module: string) => validatorModules.includes(module);

// A validator class row (kind validator, format 0.1), as listed by the server.
export type ValidatorClass = JsonObject & {
	descriptor_id: string;
	code_module_id: string;
	category: string;
	description?: Text;
	constraints: Record<string, ConstraintDef>;
	error_codes?: Record<string, { params?: string[] }>;
};

// One constraint of a class row: the shape of an element attribute.
export type ConstraintDef = JsonObject & {
	type?: string;
	choices?: string[];
	help_tip?: Text;
	default_value?: Json;
	instance_modifiable?: boolean;
	value_editor?: { id: string; options?: JsonObject };
};

export type ValidatorInstance = {
	validator: string;
	class?: ValidatorClass;
	constraints?: JsonObject;
	messages?: Record<string, Text>;
};

// The module of an instance and the constraints its copy and the instance give,
// the instance's winning, except constraints the class fixes.
export function validatorConstraints(item: ValidatorInstance): { module: string; constraints: JsonObject } {
	const defs = item.class?.constraints ?? {};
	const constraints: JsonObject = {};
	for (const [name, def] of Object.entries(defs))
		if (def.default_value !== undefined && def.default_value !== null) constraints[name] = def.default_value;
	Object.assign(constraints, item.constraints ?? {});
	for (const [name, def] of Object.entries(defs)) if (def.instance_modifiable === false) constraints[name] = def.default_value as Json;
	return { module: String(item.class?.code_module_id ?? ''), constraints };
}

// Checks value with every validator of an element; each error names its
// validator's index in the list.
export function runValidators(list: ValidatorInstance[] | undefined, value: Json | undefined, path: string): ValidationError[] {
	return (list ?? []).flatMap((item, i) => {
		const { module, constraints } = validatorConstraints(item);
		return runValidator(module, constraints, value, path).errors.map((e) => ({ ...e, validator: i }));
	});
}

// The JSON Pointer of a binding such as $input_doc.address.city: /address/city.
export function bindingPointer(binding: string | undefined): string {
	if (!binding?.startsWith('$input_doc.')) return '';
	return binding
		.slice('$input_doc.'.length)
		.split('.')
		.map((s) => '/' + s.replace(/~/g, '~0').replace(/\//g, '~1'))
		.join('');
}

// A new instance of a class, holding its copy, as a drop or "Add validator" makes.
// The classes the editor lists are Svelte state proxies, which structuredClone
// refuses; a class row is JSON, so it is copied as JSON.
export function newValidatorInstance(cls: ValidatorClass): ValidatorInstance {
	return { validator: cls.descriptor_id, class: JSON.parse(JSON.stringify(cls)) as ValidatorClass };
}
