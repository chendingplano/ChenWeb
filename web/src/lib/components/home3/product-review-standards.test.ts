import { describe, expect, test } from 'bun:test';
import { classifyStandard, groupStandardMetrics, normalizeStandardOrder, moveStandardCategory, standardCategories } from './product-review-standards';

describe('Product Review list order', () => {
	test('restores unique known categories and includes missing categories', () => {
		expect(normalizeStandardOrder(['other', 'other', 'removed', 'national'])).toEqual([
			'other', 'national', 'international', 'eu', 'us', 'industry', 'local', 'association', 'enterprise'
		]);
		for (const invalid of [null, {}, 42, 'national']) {
			expect(normalizeStandardOrder(invalid)).toEqual([...standardCategories]);
		}
	});
	test('moves across row boundaries in both directions without losing lists', () => {
		const forward = moveStandardCategory([...standardCategories], 'national', 'local');
		expect(forward).toEqual(['international', 'eu', 'us', 'industry', 'local', 'national', 'association', 'enterprise', 'other']);
		expect(moveStandardCategory(forward, 'national', 'international')).toEqual([...standardCategories]);
		expect(moveStandardCategory(forward, 'national', 'national')).toEqual(forward);
	});
});

describe('Product Review standard classification', () => {
	for (const [docNo, category] of [
		['GB 9706.1-2020', 'national'], ['GB/T 19001-2016', 'national'],
		[' gb / z 1-2026 ', 'national'], ['ISO/IEC 27001:2022', 'international'],
		['EN ISO 13485:2016', 'eu'], ['ASTM D638-22', 'us'],
		['YY/T 1474-2016', 'industry'], ['HJ/T 1-2026', 'industry'],
		['DB11/T 123-2026', 'local'], ['DB330522/T 067-2017', 'local'],
		['T/CSTM 001-2026', 'association'], ['Q/DEMO 001-2026', 'enterprise'],
		['Q／ACME 001-2026', 'enterprise'], ['', 'other'],
		['GBR 123', 'other'], ['ENGLISH 123', 'other'], ['DBJ 123', 'other'],
		['unrecognized', 'other']
	]) {
		test(`${docNo || '(missing)'} → ${category}`, () => {
			expect(classifyStandard(docNo)).toBe(category);
		});
	}
	test('keeps empty columns, metric order, and missing document numbers', () => {
		const rows = [
			{ artifact_type: 'metric', doc_no: 'GB/T 1-2026', artifact_id: 'a' },
			{ artifact_type: 'metric', artifact_id: 'b' },
			{ artifact_type: 'provision', doc_no: 'GB/T 1-2026', artifact_id: 'c' },
			{ artifact_type: 'metric', doc_no: 'GB 2-2026', artifact_id: 'd' }
		];
		const groups = groupStandardMetrics(rows);
		expect(groups).toHaveLength(9);
		expect(groups[0].rows.map(r => r.artifact_id)).toEqual(['a', 'd']);
		expect(groups.find(g => g.category === 'international')?.rows).toEqual([]);
		expect(groups.at(-1)?.rows.map(r => r.artifact_id)).toEqual(['b']);
	});
});
