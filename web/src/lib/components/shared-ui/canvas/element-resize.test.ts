import { test } from 'node:test';
import { strict as assert } from 'node:assert';
import { resizeElement, resizeHandles, resizeOutline } from './element-resize';

test('midpoint handles require enough screen space along their edge', () => {
 assert.deepEqual(resizeHandles(100, 20, 1), ['nw', 'ne', 'sw', 'se', 'n', 's']);
 assert.deepEqual(resizeHandles(100, 100, 0.2), ['nw', 'ne', 'sw', 'se']);
});

test('handles resize their axes with zoom and a minimum size', () => {
 assert.deepEqual(resizeElement({ width: 100, height: 50 }, 'nw', 20, -10, 2), { width: 90, height: 55 });
 assert.deepEqual(resizeElement({ width: 100, height: 50 }, 'e', -500, 20, 1), { width: 20 });
});


test('outline follows the dragged edges while the opposite edges stay fixed', () => {
 assert.deepEqual(resizeOutline({ x: 10, y: 20, width: 100, height: 50 }, 'nw', { width: 80, height: 70 }), { x: 30, y: 0, width: 80, height: 70 });
 assert.deepEqual(resizeOutline({ x: 10, y: 20, width: 100, height: 50 }, 'e', { width: 130 }), { x: 10, y: 20, width: 130, height: 50 });
});
