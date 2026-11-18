import { describe, expect, test } from 'bun:test';
import { validateRelationTypeInput } from './product-relation-types-client';

describe('validateRelationTypeInput', () => {
	test('requires both descriptions', () => {
		expect(validateRelationTypeInput({ description_en: 'a', description_zh: ' ', weight: 0.5 })).toBe('descriptions');
		expect(validateRelationTypeInput({ description_en: '', description_zh: 'b', weight: 0.5 })).toBe('descriptions');
	});
	test('weight must be within 0..1', () => {
		expect(validateRelationTypeInput({ description_en: 'a', description_zh: 'b', weight: 1.2 })).toBe('weight');
		expect(validateRelationTypeInput({ description_en: 'a', description_zh: 'b', weight: Number.NaN })).toBe('weight');
		expect(validateRelationTypeInput({ description_en: 'a', description_zh: 'b', weight: 0 })).toBeNull();
	});
});
