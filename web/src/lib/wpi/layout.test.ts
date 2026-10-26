import test from 'node:test';
import assert from 'node:assert/strict';
import { clampPx, flowItemStyle, isResizable, paneStyle } from './layout';

test('flow item sizes', () => {
	assert.equal(flowItemStyle({ width: '160px' }), 'flex: 0 0 160px; width: 160px');
	assert.equal(flowItemStyle({ width: 'elastic', min_width: '200px', max_width: '400px' }), 'flex: 1 1 200px; min-width: 200px; max-width: 400px');
	assert.equal(flowItemStyle({ width: 'elastic' }), 'flex: 1 1 0px');
	assert.equal(flowItemStyle({ width: 'auto' }), 'flex: 0 0 auto');
	assert.equal(flowItemStyle({}), 'flex: 0 0 auto');
	assert.equal(flowItemStyle({ width: '100%', height: '48px' }), 'flex: 0 0 100%; width: 100%; height: 48px');
});

test('pane sizes', () => {
	assert.equal(paneStyle({ width: '240px', min_width: '160px' }, 'width'), 'flex: 0 0 240px; min-width: 160px; overflow: auto');
	assert.equal(paneStyle({ width: '240px' }, 'width', 300), 'flex: 0 0 300px; overflow: auto');
	assert.equal(paneStyle({ width: 'elastic', scroll: 'none' }, 'width'), 'flex: 1 1 0px; overflow: hidden');
	assert.equal(paneStyle({ height: '48px' }, 'height'), 'flex: 0 0 48px; overflow: auto');
});

test('only fixed-pixel panes are drag-resizable, within min/max', () => {
	assert.equal(isResizable({ width: '240px', resizable: true }), true);
	assert.equal(isResizable({ width: 'elastic', resizable: true }), false);
	assert.equal(isResizable({ width: '240px' }), false);
	assert.equal(clampPx(100, '160px', '400px'), 160);
	assert.equal(clampPx(500, '160px', '400px'), 400);
	assert.equal(clampPx(250), 250);
});
