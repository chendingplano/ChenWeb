import { describe, expect, test } from 'bun:test';
import {
	filterProductRelationTypes,
	validateRelationTypeInput,
	type ProductRelationType
} from './product-relation-types-client';

const row = (over: Partial<ProductRelationType>): ProductRelationType => ({
	code: 'pkg', value: 'pkg', status: 'proposed', description_en: '', description_zh: '',
	weight: 0.5, first_record_id: 377, first_product_ref: '', seen_count: 1, product_count: 0,
	extra_info: {}, created_at: '', approved_at: null, approved_by: '', ...over
});

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

describe('filterProductRelationTypes', () => {
	const rows = [row({}), row({ code: 'stor', value: 'storage_requirement', status: 'approved', description_en: 'Storage' })];
	test('filters by status and search', () => {
		expect(filterProductRelationTypes(rows, 'proposed', '').map((r) => r.code)).toEqual(['pkg']);
		expect(filterProductRelationTypes(rows, 'all', 'storage').map((r) => r.code)).toEqual(['stor']);
	});
});
