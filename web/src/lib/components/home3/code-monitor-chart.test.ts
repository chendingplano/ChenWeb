import { describe, expect, test } from 'bun:test';
import { codeMonitorTotalBaseline } from './code-monitor-chart';

describe('codeMonitorTotalBaseline', () => {
	test('makes changes in large totals visible using a rounded baseline', () => {
		const totals = [780_000, 790_000, 810_000];
		const baseline = codeMonitorTotalBaseline(totals);
		expect(baseline).toBe(770_000);
		expect(totals.map((total) => total - baseline)).toEqual([10_000, 20_000, 40_000]);
	});

	test('uses the maximum to set rounding precision without clipping smaller totals', () => {
		expect(codeMonitorTotalBaseline([54_010, 54_200])).toBe(54_000);
		expect(codeMonitorTotalBaseline([54_010, 810_000])).toBe(50_000);
		expect(codeMonitorTotalBaseline([5, 810_000])).toBe(0);
	});

	test('keeps flat and single-snapshot totals visible', () => {
		expect(codeMonitorTotalBaseline([180_000, 180_000])).toBe(170_000);
		expect(codeMonitorTotalBaseline([5_000])).toBe(4_900);
		expect(codeMonitorTotalBaseline([1])).toBe(0);
	});

	test('uses zero for empty history or a range containing zero', () => {
		expect(codeMonitorTotalBaseline([])).toBe(0);
		expect(codeMonitorTotalBaseline([0])).toBe(0);
		expect(codeMonitorTotalBaseline([100, 0, 200])).toBe(0);
	});
});
