import test from 'node:test';
import assert from 'node:assert/strict';
import { m } from '$lib/paraglide/messages.js';
import { constraintLabel, validationErrorText } from './messages';
import { modules } from './run';

const sample = { min_length: 3, max_length: 3, length_unit: 'code_points', actual: 2, limit: 10000, prefix: 'a', suffix: 'b', contains: 'c', minimum: 1, maximum: 2, exclusive_minimum: 1, exclusive_maximum: 2, sign: 'positive', multiple_of: 5, lowest: '0', highest: '255', scale: 2, precision: 5, integer_digits: 3, must_be: 'true', encoding: 'hex', padding: 'required', hex_case: 'lower', min_bytes: 1, max_bytes: 2, min_digits: 1, max_digits: 2, allowed_values: ['a'], excluded_values: ['b'], pattern: 'x' };

test('every error code of every module has its own text', () => {
	for (const [id, mod] of Object.entries(modules))
		for (const code of Object.keys(mod.error_codes)) {
			const text = validationErrorText({ code, path: '', params: sample, validator: 0 });
			assert.notEqual(text, m.wpi_vld_e_unknown({ code }), `${id}: ${code}`);
		}
});

test('every constraint has a label', () => {
	for (const mod of Object.values(modules)) for (const c of mod.constraints) assert.notEqual(constraintLabel(c.name), c.name, c.name);
});

test("an instance's own message replaces the text, with params filled in", () => {
	const error = { code: 'integer.maximum', path: '/age', params: { maximum: 150 }, validator: 0 };
	assert.equal(validationErrorText(error, { validator: 'integer', messages: { 'integer.maximum': 'Age is at most {maximum} {unknown}.' } }), 'Age is at most 150 {unknown}.');
	assert.equal(validationErrorText(error, { validator: 'integer', messages: { 'integer.minimum': 'x' } }), m.wpi_vld_e_maximum({ maximum: '150' }));
	assert.equal(validationErrorText({ code: 'string.min_length', path: '', params: { min_length: 2, length_unit: 'utf8_bytes', actual: 1 }, validator: 0 }), m.wpi_vld_e_min_length_utf8_bytes({ min_length: '2', actual: '1' }));
});
