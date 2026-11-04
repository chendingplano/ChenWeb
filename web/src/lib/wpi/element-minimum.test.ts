import { test, expect } from 'bun:test';
import { elementMinimum, belowElementMinimum } from './element-minimum';
test('color picker dimensions reject sizes below its content minimum', () => {
 const min = elementMinimum('value-editor', 'color');
 expect(belowElementMinimum({ width: 279 }, min)).toBe(true);
 expect(belowElementMinimum({ height: 119 }, min)).toBe(true);
 expect(belowElementMinimum({ width: 280, height: 120 }, min)).toBe(false);
 expect(elementMinimum('value-editor', 'text')).toBeUndefined();
});
