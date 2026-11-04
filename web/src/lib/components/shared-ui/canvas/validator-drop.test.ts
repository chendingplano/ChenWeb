import { expect, test } from 'bun:test';
import type { Node } from '@xyflow/svelte';
import type { ElementClassSummary } from '$lib/wpi/api';
import type { ValidatorClass } from '$lib/wpi/validators';
import { addValidator, elementModule } from './validator-drop';
import { VALIDATOR_CLASS_PREFIX, validatorPaletteTypes } from './element-palette';

const node = (id: string, data: Record<string, unknown>): Node => ({ id, type: 'wpi-element', position: { x: 0, y: 0 }, data });
const integer = { descriptor_id: 'integer', code_module_id: 'integer', category: 'primitive', constraints: {} } as unknown as ValidatorClass;

test('a dropped validator is appended with a copy of its class, and its element selected', () => {
	const nodes = [
		node('a', { element: 'input', attrs: { label: 'Age', validators: [{ validator: 'string', class: {} }] } }),
		{ ...node('b', { element: 'text' }), selected: true }
	];
	const next = addValidator(nodes, 'a', integer);
	const list = (next[0].data.attrs as { validators: { validator: string; class: unknown }[] }).validators;
	expect(list.map((v) => v.validator)).toEqual(['string', 'integer']);
	expect(list[1].class).toEqual(integer);
	expect(list[1].class).not.toBe(integer);
	expect((next[0].data.attrs as { label: string }).label).toBe('Age');
	expect(next.map((n) => !!n.selected)).toEqual([true, false]);
});

test('the module of an element node comes from its copy, else its listed class', () => {
	const classes = [{ descriptor_id: 'age-input', code_module_id: 'input' }] as ElementClassSummary[];
	expect(elementModule(node('a', { element: 'age-input' }), classes)).toBe('input');
	expect(elementModule(node('a', { element: 'x', class: { code_module_id: 'select' } }), classes)).toBe('select');
	expect(elementModule(node('a', { element: 'unknown' }), classes)).toBe('');
});

test('validators are listed by category order, then ID, in sub-folders', () => {
	const cls = (descriptor_id: string, category: string) => ({ descriptor_id, category, code_module_id: descriptor_id, constraints: {} }) as unknown as ValidatorClass;
	const types = validatorPaletteTypes([cls('email', 'network'), cls('string', 'primitive'), cls('odd', 'nowhere'), cls('bigint', 'primitive')], 'tip');
	expect(types.map((t) => [t.id, t.category, t.subcategory, t.title])).toEqual([
		[VALIDATOR_CLASS_PREFIX + 'bigint', 'Validators', 'primitive', 'tip'],
		[VALIDATOR_CLASS_PREFIX + 'string', 'Validators', 'primitive', 'tip'],
		[VALIDATOR_CLASS_PREFIX + 'email', 'Validators', 'network', 'tip'],
		[VALIDATOR_CLASS_PREFIX + 'odd', 'Validators', 'nowhere', 'tip']
	]);
});
