import test from 'node:test';
import assert from 'node:assert/strict';
import { clampPx, flowItemStyle, flowStyle, isResizable, pageFrameStyle, pageStyle, paneStyle } from './layout';
import type { PageDescriptor } from './types';

test('flow defaults preserve content height and start alignment', () => {
	const css = flowStyle({});
	assert.match(css, /column-gap: 0.75rem/);
	assert.match(css, /row-gap: 0.75rem/);
	assert.match(css, /justify-content: start/);
	assert.match(css, /align-items: flex-start/);
	assert.match(css, /align-content: flex-start/);
	assert.doesNotMatch(css, /height:/);
});

test('flow configuration maps both alignment axes and propagates definite height', () => {
	const css = flowStyle({ height: '400px', layouter_config: {
		column_gap: '0px', row_gap: '2rem', horizontal_alignment: 'end',
		vertical_alignment: 'stretch', rows_alignment: 'bottom'
	} });
	assert.match(css, /column-gap: 0px/);
	assert.match(css, /row-gap: 2rem/);
	assert.match(css, /justify-content: end/);
	assert.match(css, /align-items: stretch/);
	assert.match(css, /align-content: safe flex-end/);
	assert.match(css, /height: 100%/);
	assert.match(flowStyle({ layouter_config: { rows_alignment: 'center', vertical_alignment: 'bottom' } }), /align-content: safe center/);
	assert.match(flowStyle({ layouter_config: { rows_alignment: 'space-between', horizontal_alignment: 'space-between' } }), /align-content: space-between/);
	assert.doesNotMatch(flowStyle({ height: 'auto' }), /height:/);
});

test('flow item sizes', () => {
	assert.equal(flowItemStyle({ width: 600, height: 300 }), 'flex: 0 0 600px; width: 600px; height: 300px');
	assert.equal(flowItemStyle({ width: 0, height: 0 }), 'flex: 0 0 0px; width: 0px; height: 0px');
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

test('page box: sizes, placement, scroll and colors', () => {
	const page = (extra: Partial<PageDescriptor>): PageDescriptor => ({ descriptor_id: 'p', schema_version: '0.2', layouter: 'flow-layouter', elements: [], ...extra });
	const base = 'flex: 0 0 auto; align-self: flex-start';
	assert.equal(pageStyle(page({ hori_align: 'left' })), `${base}; width: 100%; min-width: 50px; min-height: 50px`);
	assert.equal(pageStyle(page({})).includes('align-self: center'), true);
	assert.equal(
		pageStyle(page({ hori_align: 'right', width: '50 %', height: '400', min_width: '320', max_height: '600px', bkgd_color: '#fff', frgd_color: 'auto' })),
		'flex: 0 0 auto; align-self: flex-end; width: 50%; height: 400px; min-width: 320px; min-height: 50px; max-height: 600px; overflow: auto; background-color: #fff'
	);
	assert.match(pageStyle(page({ max_width: '640px' })), /max-width: 640px; overflow: auto$/);
});

test('page frame: margins default to 30px and place the page vertically', () => {
	const page = (extra: Partial<PageDescriptor>): PageDescriptor => ({ descriptor_id: 'p', schema_version: '0.2', layouter: 'flow-layouter', elements: [], ...extra });
	assert.equal(pageFrameStyle(page({})), 'display: flex; flex-direction: column; box-sizing: border-box; justify-content: flex-start; padding: 30px 30px 30px 30px');
	assert.equal(
		pageFrameStyle(page({ vert_align: 'bottom', top_margin: '1.5 rem', left_margin: '8' })),
		'display: flex; flex-direction: column; box-sizing: border-box; justify-content: flex-end; padding: 1.5rem 30px 30px 8px'
	);
});
