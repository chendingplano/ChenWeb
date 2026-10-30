import { test, expect } from 'bun:test';
import { validateInput } from './input-validation';

test('validates required, Unicode length and numeric constraints', () => {
  expect(validateInput('', { required: true })).toEqual(['required']);
  expect(validateInput('😀a', { max_length: 1 })).toEqual(['max_length']);
  expect(validateInput('', { validation: { min_length: 3 } })).toEqual([]);
  expect(validateInput('5', { input_type: 'number', validation: { min: 2, step: 2 } })).toEqual(['step']);
  expect(validateInput('6', { input_type: 'number', validation: { min: 2, step: 2 } })).toEqual([]);
});

test('honors read-only, optional empty values, format and whole-string patterns', () => {
  expect(validateInput('', { required: true, binding_mode: 'read-only' })).toEqual([]);
  expect(validateInput('', { input_type: 'email' })).toEqual([]);
  expect(validateInput('a@b', { input_type: 'email' })).toEqual(['format']);
  expect(validateInput('https://example.com', { input_type: 'url' })).toEqual([]);
  expect(validateInput('abc\n', { validation: { pattern: '[a-z]+' } })).toEqual(['pattern']);
  expect(validateInput('😀', { validation: { min_length: 2 } })).toEqual(['min_length']);
});
