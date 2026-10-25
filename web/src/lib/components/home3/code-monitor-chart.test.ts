import { describe, expect, test } from 'bun:test';
import { codeMonitorTotalScale } from './code-monitor-chart';

describe('codeMonitorTotalScale', () => {
	test('chooses independent baselines and intervals for each metric', () => {
		const scales = [[785_000, 821_000], [151_000, 161_000], [54_010, 54_210]].map(codeMonitorTotalScale);
		expect(new Set(scales.map((scale) => scale.baseline)).size).toBe(3);
		expect(new Set(scales.map((scale) => scale.interval)).size).toBe(3);
	});

	for (const totals of [[785_000, 821_000], [151_000, 161_000], [54_010, 54_210], [4_999, 4_999], [180_123], [0], [], [1], [0, 200], [5, 810_000]]) {
		test(`encloses ${JSON.stringify(totals)} with about eight gridlines`, () => {
			const { baseline, interval, maximum } = codeMonitorTotalScale(totals);
			expect(baseline).toBeGreaterThanOrEqual(0);
			expect(interval).toBeGreaterThanOrEqual(100);
			expect(interval % 100).toBe(0);
			expect(baseline % 100).toBe(0);
			expect(maximum % 100).toBe(0);
			expect(Number.isInteger(interval)).toBe(true);
			expect(maximum / interval + 1).toBeGreaterThanOrEqual(6);
			expect(maximum / interval + 1).toBeLessThanOrEqual(10);
			for (const total of totals) {
				if (total > 0) expect(total - baseline).toBeGreaterThan(0);
				expect(total - baseline).toBeLessThan(maximum);
			}
		});
	}
});
