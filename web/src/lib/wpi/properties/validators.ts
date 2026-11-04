// The Validators group of an element's property editor (OpenSpec
// wpi-primitive-validators, design D9, D10): the validators list, then one group
// per validator whose properties are its constraints, drawn from the class copy
// the instance holds and the code module's constraint specs; no Svelte per validator.
import { m } from '$lib/paraglide/messages.js';
import type { ValueEditorOptions } from '$lib/components/shared-ui/value-editors/types';
import { localized } from '../text';
import type { Json, JsonObject } from '../types';
import { integerSyntax, numberDec, sign } from '../validators/decimal';
import { modules, type ConstraintDef, type ValidatorInstance } from '../validators';
import { choiceLabel, constraintLabel } from '../validators/messages';
import { patternProblem } from '../validators/text';
import { drawingEditor } from './editor-ids';
import type { EditorDef, GroupDef, PropertyDef, PropertySource } from './types';

export const validatorsGroup: GroupDef = {
	id: 'validators',
	label: m.wpi_vld_group_validators,
	help: m.wpi_vld_group_validators_help,
	properties: [
		{
			name: 'validators',
			label: m.wpi_vld_attr_validators,
			editor: 'validator-list',
			options: (ctx) => ({ validatorClasses: ctx.validatorClasses ?? [] })
		}
	]
};

// A constraint property's name: validators.<index>.<constraint>, or .messages.
const PREFIX = 'validators.';

function parseName(name: string): { index: number; key: string } | null {
	const match = /^validators\.(\d+)\.(\w+)$/.exec(name);
	return match ? { index: Number(match[1]), key: match[2] } : null;
}

function asList(value: Json | undefined): ValidatorInstance[] {
	return Array.isArray(value) ? (value as unknown as ValidatorInstance[]) : [];
}

function decimalCheck(positive: boolean, whole: boolean) {
	return (v: Json) => {
		const d = typeof v === 'string' && (!whole || integerSyntax.test(v)) ? numberDec(v) : typeof v === 'number' ? numberDec(v) : null;
		if (!d || (whole && d.exp < 0)) return whole ? m.wpi_vld_invalid_bigint() : m.wpi_vld_invalid_decimal();
		return positive && sign(d) <= 0 ? m.wpi_vld_invalid_positive() : null;
	};
}

// The value editor of a constraint, by its type (design D10), unless the copy
// configures one.
function constraintEditor(name: string, type: string, choices: string[] | undefined, def: ConstraintDef): Omit<PropertyDef, 'name'> {
	const label = () => constraintLabel(name);
	const configured = def.value_editor && drawingEditor(def.value_editor.id);
	if (configured) return { label, editor: configured, options: (def.value_editor?.options ?? {}) as ValueEditorOptions };
	switch (type) {
		case 'boolean':
			return { label, editor: 'switch' };
		case 'integer':
			return { label, editor: 'number', options: { integer: true } };
		case 'non_negative_integer':
			return { label, editor: 'number', options: { integer: true, min: 0 } };
		case 'positive_integer':
			return { label, editor: 'number', options: { integer: true, min: 1 } };
		case 'number':
			return { label, editor: 'number' };
		case 'positive_number':
			return { label, editor: 'number', validate: (v) => (typeof v === 'number' && v > 0 ? null : m.wpi_vld_invalid_positive()) };
		case 'decimal':
		case 'positive_decimal':
			return { label, editor: 'text', validate: decimalCheck(type === 'positive_decimal', false) };
		case 'bigint':
		case 'positive_bigint':
			return { label, editor: 'text', validate: decimalCheck(type === 'positive_bigint', true) };
		case 'pattern':
			return {
				label,
				editor: 'text',
				validate: (v) => {
					const problem = typeof v === 'string' ? patternProblem(v) : 'invalid';
					return problem === 'invalid' ? m.wpi_vld_invalid_pattern() : problem ? m.wpi_vld_invalid_pattern_portable() : null;
				}
			};
		case 'choice':
			return { label, editor: 'value-picker', options: { choices: choices ?? [], choiceLabel: (c) => choiceLabel(name, c) } };
		case 'json_array':
			return { label, editor: 'json', options: { rows: 2 }, validate: (v) => (Array.isArray(v) ? null : m.wpi_vld_invalid_array()) };
		case 'string':
			return { label, editor: 'text' };
	}
	return { label, editor: 'json' };
}

// One group per validator of the element, after the Validators group: each
// constraint its copy lists, in the order of the code, then its messages.
export function withValidatorGroups(def: EditorDef, validators: Json | undefined): EditorDef {
	const list = asList(validators);
	if (!list.length) return def;
	const groups: GroupDef[] = list.map((item, index) => {
		const copy = item.class;
		const defs = copy?.constraints ?? {};
		const specs = modules[String(copy?.code_module_id ?? '')]?.constraints ?? [];
		const names = [...specs.map((s) => s.name).filter((n) => n in defs), ...Object.keys(defs).filter((n) => !specs.some((s) => s.name === n))];
		const codes = Object.keys(copy?.error_codes ?? {});
		const properties: PropertyDef[] = names.map((name) => {
			const spec = specs.find((s) => s.name === name);
			return { name: `${PREFIX}${index}.${name}`, ...constraintEditor(name, spec?.type ?? String(defs[name].type ?? ''), spec?.choices ?? defs[name].choices, defs[name]) };
		});
		properties.push({
			name: `${PREFIX}${index}.messages`,
			label: m.wpi_vld_attr_messages,
			editor: 'json',
			options: { rows: 3 },
			helpTip: () => m.wpi_vld_messages_tip({ example: `{"${codes[0] ?? 'integer.maximum'}": {"en": "…", "zh-cn": "…"}}`, codes: codes.join(', ') }),
			validate: (v) => (v && typeof v === 'object' && !Array.isArray(v) ? null : m.wpi_vld_invalid_messages())
		});
		return {
			id: `validator-${index}`,
			label: () => m.wpi_vld_group({ number: String(index + 1), name: item.validator }),
			help: () => localized(copy?.description),
			properties
		};
	});
	const at = def.groups.findIndex((g) => g.id === validatorsGroup.id);
	const next = [...def.groups];
	next.splice(at < 0 ? next.length : at + 1, 0, ...groups);
	return { ...def, groups: next };
}

// The element's source, also serving the constraint properties of its validators:
// their values come from each instance, their tips, defaults and fixed or
// required marks from its copy, and the code default shows when nothing is set.
export function withValidatorSource(base: PropertySource): PropertySource {
	const at = (name: string) => {
		const parsed = parseName(name);
		if (!parsed) return null;
		const list = asList(base.get('validators'));
		const item = list[parsed.index];
		if (!item) return null;
		const def = parsed.key === 'messages' ? undefined : item.class?.constraints?.[parsed.key];
		const spec = modules[String(item.class?.code_module_id ?? '')]?.constraints.find((s) => s.name === parsed.key);
		return { ...parsed, list, item, def, spec };
	};
	const fallback = (p: NonNullable<ReturnType<typeof at>>): Json | undefined => {
		if (p.key === 'messages') return undefined;
		const dv = p.def?.default_value;
		return dv !== undefined && dv !== null ? dv : (p.spec?.default as Json | undefined);
	};
	return {
		...base,
		get: (name) => {
			const p = at(name);
			if (!p) return base.get(name);
			if (p.key === 'messages') return p.item.messages as Json | undefined;
			if (p.def?.instance_modifiable === false) return p.def.default_value;
			return p.item.constraints?.[p.key] ?? fallback(p);
		},
		isSet: (name) => {
			const p = at(name);
			if (!p) return base.isSet(name);
			return p.key === 'messages' ? p.item.messages !== undefined : !!p.item.constraints && p.key in p.item.constraints;
		},
		fixed: (name) => {
			const p = at(name);
			return p ? p.def?.instance_modifiable === false : base.fixed(name);
		},
		required: (name) => {
			const p = at(name);
			return p ? p.def?.default_value === null : base.required(name);
		},
		helpTip: (name) => {
			const p = at(name);
			return p ? localized(p.def?.help_tip) : base.helpTip(name);
		},
		defaultValue: (name) => {
			const p = at(name);
			return p ? fallback(p) : base.defaultValue(name);
		},
		set: (name, value) => {
			const p = at(name);
			if (!p) return base.set(name, value);
			if (p.key === 'messages') {
				const item: ValidatorInstance = { ...p.item };
				if (value === undefined) delete item.messages;
				else item.messages = value as ValidatorInstance['messages'];
				return base.set('validators', p.list.map((v, i) => (i === p.index ? item : v)) as unknown as Json);
			}
			const values: JsonObject = { ...(p.item.constraints ?? {}) };
			if (value === undefined) delete values[p.key];
			else values[p.key] = value;
			const item: ValidatorInstance = { ...p.item, constraints: values };
			if (!Object.keys(values).length) delete item.constraints;
			base.set('validators', p.list.map((v, i) => (i === p.index ? item : v)) as unknown as Json);
		},
		resettable: (name) => (name.startsWith(PREFIX) ? true : base.resettable(name))
	};
}
