import { test, expect } from 'bun:test';
import { elementMinimum, belowElementMinimum, clampElementMinimum } from './element-minimum';
test('color picker dimensions reject sizes below its content minimum', () => {
 const min = elementMinimum('value-editor', 'color');
 expect(belowElementMinimum({ width: 199 }, min)).toBe(true);
 expect(belowElementMinimum({ height: 119 }, min)).toBe(true);
 expect(belowElementMinimum({ width: 200, height: 120 }, min)).toBe(false);
 expect(elementMinimum('value-editor', 'text')).toBeUndefined();
 expect(clampElementMinimum({ width: 200, height: 180 }, min)).toEqual({ width: 200, height: 180 });
 expect(clampElementMinimum({ height: 20 }, min)).toEqual({ height: 120 });
 expect(clampElementMinimum({ width: 20 }, undefined)).toEqual({ width: 20 });
});
