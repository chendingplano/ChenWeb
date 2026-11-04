import test from 'node:test';
import assert from 'node:assert/strict';
import type { Json, JsonObject } from '../types';
import type { ValidatorClass } from '../validators';
import { elementEditor, moduleFallbacks } from './index';
import { elementSource } from './sources';
import { validatorsGroup, withValidatorGroups, withValidatorSource } from './validators';

const integer = {
	descriptor_id: 'integer',
	code_module_id: 'integer',
	category: 'primitive',
	description: { en: 'A whole number' },
	error_codes: { 'integer.maximum': { params: ['maximum'] } },
	constraints: {
		maximum: { type: 'integer', help_tip: { en: 'The largest value' } },
		empty: { type: 'choice', choices: ['pass', 'fail'], default_value: 'pass' },
		minimum: { type: 'integer', default_value: 0, instance_modifiable: false },
		sign: { type: 'choice' }
	}
} as unknown as ValidatorClass;

function setup(attrs: JsonObject) {
	let current = attrs;
	const make = () =>
		withValidatorSource(elementSource(current, null, moduleFallbacks('input'), (next) => (current = next)));
	return { source: () => make(), attrs: () => current };
}

test('input, select and value-editor definitions have the Validators group', () => {
	for (const module of ['input', 'select', 'value-editor'])
		assert.ok(elementEditor(module).groups.some((g) => g.id === validatorsGroup.id), module);
	assert.ok(!elementEditor('button').groups.some((g) => g.id === validatorsGroup.id));
});

test('each validator gets a group of its constraints in the code order, then messages', () => {
	const def = withValidatorGroups(elementEditor('input'), [{ validator: 'integer', class: integer }] as unknown as Json);
	const at = def.groups.findIndex((g) => g.id === 'validators');
	const group = def.groups[at + 1];
	assert.equal(group.id, 'validator-0');
	assert.deepEqual(
		group.properties.map((p) => [p.name, p.editor]),
		[
			['validators.0.empty', 'value-picker'],
			['validators.0.minimum', 'number'],
			['validators.0.maximum', 'number'],
			['validators.0.sign', 'value-picker'],
			['validators.0.messages', 'json']
		]
	);
	// Choices come from the code when the copy does not restate them.
	const sign = group.properties.find((p) => p.name === 'validators.0.sign')!;
	assert.deepEqual((sign.options as { choices: string[] }).choices, ['any', 'positive', 'negative', 'non_negative', 'non_positive', 'non_zero']);
	const plain = elementEditor('input');
	assert.equal(withValidatorGroups(plain, undefined), plain);
});

test('constraint values are read and written on the instance, with tips and marks from the copy', () => {
	const s = setup({ label: 'Age', validators: [{ validator: 'integer', class: integer }] as unknown as Json });
	assert.equal(s.source().get('validators.0.empty'), 'pass');
	assert.equal(s.source().get('validators.0.trim'), undefined);
	assert.equal(s.source().helpTip('validators.0.maximum'), 'The largest value');
	assert.ok(s.source().fixed('validators.0.minimum'));
	assert.equal(s.source().get('validators.0.minimum'), 0);
	assert.ok(!s.source().isSet('validators.0.maximum'));
	s.source().set('validators.0.maximum', 150);
	assert.deepEqual((s.attrs().validators as JsonObject[])[0].constraints, { maximum: 150 });
	assert.ok(s.source().isSet('validators.0.maximum'));
	assert.equal(s.source().get('validators.0.maximum'), 150);
	s.source().set('validators.0.messages', { 'integer.maximum': 'At most {maximum}' });
	assert.deepEqual((s.attrs().validators as JsonObject[])[0].messages, { 'integer.maximum': 'At most {maximum}' });
	s.source().set('validators.0.maximum', undefined);
	assert.ok(!('constraints' in (s.attrs().validators as JsonObject[])[0]));
	assert.equal(s.attrs().label, 'Age');
	// Other attributes pass through to the element's source.
	s.source().set('label', 'Years');
	assert.equal(s.attrs().label, 'Years');
});

test('decimal, bigint and pattern constraints refuse values the code cannot use', () => {
	const decimal = { ...integer, code_module_id: 'decimal', constraints: { minimum: { type: 'decimal' }, pattern: { type: 'pattern' } } } as unknown as ValidatorClass;
	const bigint = { ...integer, code_module_id: 'bigint', constraints: { maximum: { type: 'bigint' }, multiple_of: { type: 'positive_bigint' } } } as unknown as ValidatorClass;
	const string = { ...integer, code_module_id: 'string', constraints: { pattern: { type: 'pattern' } } } as unknown as ValidatorClass;
	const props = withValidatorGroups({ groups: [validatorsGroup] }, [
		{ validator: 'decimal', class: decimal },
		{ validator: 'bigint', class: bigint },
		{ validator: 'string', class: string }
	] as unknown as Json).groups.flatMap((g) => g.properties);
	const check = (name: string, v: Json) => props.find((p) => p.name === name)!.validate!(v, {} as never);
	assert.equal(check('validators.0.minimum', '0.01'), null);
	assert.notEqual(check('validators.0.minimum', 'ten'), null);
	assert.equal(check('validators.1.maximum', '9007199254740993'), null);
	assert.notEqual(check('validators.1.maximum', '1.5'), null);
	assert.notEqual(check('validators.1.multiple_of', '0'), null);
	assert.equal(check('validators.2.pattern', '[a-z]+'), null);
	assert.notEqual(check('validators.2.pattern', '(?i)a'), null);
	assert.notEqual(check('validators.2.pattern', '\\p{L}'), null);
});
