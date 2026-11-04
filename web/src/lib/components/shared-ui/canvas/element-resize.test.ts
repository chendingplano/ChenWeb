import { test } from 'node:test';
import { strict as assert } from 'node:assert';
import { resizeElement, resizeHandles } from './element-resize';

test('midpoint handles require enough screen space along their edge', () => {
 assert.deepEqual(resizeHandles(100, 20, 1), ['nw', 'ne', 'sw', 'se', 'n', 's']);
 assert.deepEqual(resizeHandles(100, 100, 0.2), ['nw', 'ne', 'sw', 'se']);
});

test('handles resize their axes with zoom and a minimum size', () => {
 assert.deepEqual(resizeElement({ width: 100, height: 50 }, 'nw', 20, -10, 2), { width: 90, height: 55 });
 assert.deepEqual(resizeElement({ width: 100, height: 50 }, 'e', -500, 20, 1), { width: 20 });
});
