import test from 'node:test';
import assert from 'node:assert/strict';
import file from '../../../../../server/api/wpi/validators/validator-cases.json';
import { bindingPointer, runValidator, runValidators, validatorConstraints, type ValidatorClass } from './index';
import { decString, parseDec } from './decimal';
import { patternProblem, trimSpace } from './text';
import type { Json, JsonObject } from '../types';

type Case = { name: string; validator: string; constraints: JsonObject; value?: Json; value_repeat?: [string, number]; errors: Json[]; normalized?: Json };

// Every shared case passes; the Go suite (validators_test.go) runs the same file.
test('validators pass the shared cases', () => {
	const cases = file.cases as unknown as Case[];
	assert.ok(cases.length >= 150);
	for (const c of cases) {
		const value = c.value_repeat ? c.value_repeat[0].repeat(c.value_repeat[1]) : 'value' in c ? c.value : undefined;
		const res = runValidator(c.validator, c.constraints, value, '/v');
		const got = res.errors.map((e) => ({ code: e.code, params: e.params }));
		assert.deepEqual(got, c.errors, c.name);
		assert.equal(res.valid, res.errors.length === 0, c.name);
		assert.ok(res.errors.every((e) => e.path === '/v'), c.name);
		if ('normalized' in c) {
			assert.ok(res.hasValue, `${c.name}: no normalized value`);
			assert.deepEqual(JSON.parse(JSON.stringify(res.value)), c.normalized, c.name);
		}
	}
});

test('decimal parsing and plain form', () => {
	for (const [text, want] of [['0.10', '0.1'], ['-0', '0'], ['1e3', '1000'], ['1.5e-7', '0.00000015'], ['.5', '0.5'], ['+12.', '12']])
		assert.equal(decString(parseDec(text)!), want, text);
	for (const bad of ['', '.', '1..2', '1e', 'e3', '0x10', '1e10001', '1'.repeat(1001)]) assert.equal(parseDec(bad), null, bad);
});

test('whitespace and patterns', () => {
	assert.equal(trimSpace('\u0085x\ufeff', 'both'), 'x');
	assert.equal(patternProblem('\\p{L}+'), 'non_portable');
	assert.equal(patternProblem('(?i)abc'), 'invalid');
	assert.equal(patternProblem('a('), 'invalid');
	assert.equal(patternProblem('[a-z]+'), null);
});

const integerClass = {
	descriptor_id: 'age',
	code_module_id: 'integer',
	category: 'datetime',
	constraints: { minimum: { default_value: 0 }, maximum: { default_value: 150, instance_modifiable: false } }
} as unknown as ValidatorClass;

test('instance constraints: copy defaults, own values, fixed values win', () => {
	assert.deepEqual(validatorConstraints({ validator: 'age', class: integerClass, constraints: { minimum: 18, maximum: 999 } }), {
		module: 'integer',
		constraints: { minimum: 18, maximum: 150 }
	});
});

test('an element runs all its validators and names each error index', () => {
	const list = [
		{ validator: 'age', class: integerClass },
		{ validator: 'age', class: integerClass, constraints: { minimum: 300 } }
	];
	const errors = runValidators(list, 200, bindingPointer('$input_doc.person.age'));
	assert.deepEqual(
		errors.map((e) => [e.code, e.validator, e.path]),
		[
			['integer.maximum', 0, '/person/age'],
			['integer.minimum', 1, '/person/age'],
			['integer.maximum', 1, '/person/age']
		]
	);
	assert.equal(bindingPointer('$input_doc.a/b'), '/a~1b');
	assert.equal(bindingPointer(undefined), '');
});
