import { test, expect } from 'bun:test';
import { hsvToRgb, rgbToHsv, rgbaHex } from './color-math';

test('color controls preserve hue and opacity', () => {
	expect(hsvToRgb(...rgbToHsv(182, 157, 248))).toEqual([182, 157, 248]);
	expect(rgbaHex(182, 157, 248, 0.5)).toBe('#b69df880');
	expect(rgbaHex(255, 0, 0, 1)).toBe('#ff0000');
});
