import test from 'node:test';
import assert from 'node:assert/strict';
import { resizePageDimension, pageDragSize } from './page-resize';

test('page edges use the measured page size and account for zoom and direction', () => {
	assert.equal(pageDragSize(500, 40, 0.5, 'right'), 580);
	assert.equal(pageDragSize(500, 40, 0.5, 'left'), 420);
	assert.equal(pageDragSize(300, -30, 1, 'top'), 330);
	assert.equal(pageDragSize(50, -100, 1, 'bottom'), 50);
});

test('resizing updates only the requested page dimension and keeps limits consistent', () => {
	assert.deepEqual(resizePageDimension({ width: '50%', height: '300px', max_width: '450px' }, 'width', 500), { width: '500px', height: '300px', max_width: '500px' });
	assert.deepEqual(resizePageDimension({ min_height: '200px' }, 'height', 100), { min_height: '100px', height: '100px' });
});
