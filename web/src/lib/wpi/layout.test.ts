import test from 'node:test';
import assert from 'node:assert/strict';
import {
	clampPx,
	surfaceColorStyle,

	cssLength,
	effectiveRoot,
	fillsHeight,
	findPane,
	flowItemStyle,
	flowStyle,
	isResizable,
	leafPanes,
	pageBoxStyle,
	panelStyle,
	paneStyle,
	placeStyle,
	splitStyle
} from './layout';
import type { PageDescriptor, PaneSpec } from './types';

const page = (extra: Partial<PageDescriptor> = {}): PageDescriptor => ({
	descriptor_id: 'p',
	schema_version: '0.5',
	page_layouter: { layouter: 'single-pane', class: { descriptor_id: 'single-pane', schema_version: '0.5', root: { pane_id: 'main' } } },
	...extra
});

const home: PaneSpec = {
	pane_id: 'page',
	split: 'rows',
	children: [
		{ pane_id: 'top', height: '48px' },
		{ pane_id: 'middle-row', split: 'columns', children: [{ pane_id: 'left', width: '240px' }, { pane_id: 'middle' }] }
	]
};

test('flow defaults preserve content height and start alignment', () => {
	const css = flowStyle({});
	assert.match(css, /column-gap: 0.75rem/);
	assert.match(css, /row-gap: 0.75rem/);
	assert.match(css, /justify-content: start/);
	assert.match(css, /align-items: flex-start/);
	assert.match(css, /align-content: flex-start/);
	assert.doesNotMatch(css, /height:/);
});

test('flow configuration maps both alignment axes', () => {
	const css = flowStyle({ column_gap: '0px', row_gap: '2rem', horizontal_alignment: 'end', vertical_alignment: 'stretch', rows_alignment: 'bottom' });
	assert.match(css, /column-gap: 0px/);
	assert.match(css, /row-gap: 2rem/);
	assert.match(css, /justify-content: end/);
	assert.match(css, /align-items: stretch/);
	assert.match(css, /align-content: safe flex-end/);
	assert.match(flowStyle({ rows_alignment: 'center', vertical_alignment: 'bottom' }), /align-content: safe center/);
	assert.match(flowStyle({ rows_alignment: 'space-between', horizontal_alignment: 'space-between' }), /align-content: space-between/);
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

test('flow constraints apply defaults and honor narrower element limits', () => {
	const css = flowItemStyle({ width: '400px', min_width: '40px', max_width: '250px' }, {});
	assert.match(css, /min-width: max\(20px, 40px\)/);
	assert.match(css, /max-width: min\(300px, 250px\)/);
	assert.match(css, /min-height: 30px/);
	assert.match(css, /max-height: 200px/);
});

test('a panel is as wide as its content, or its pane with elastic or percentage elements', () => {
	assert.match(panelStyle([{ width: '160px' }, {}]), /max-width: 100%/);
	assert.match(panelStyle([{ width: 'elastic' }]), /width: 100%/);
	assert.match(panelStyle([{ width: '50%' }]), /width: 100%/);
});

test('trees: effective root, leaves, lookup', () => {
	assert.deepEqual(effectiveRoot(page()), { pane_id: 'main' });
	assert.equal(effectiveRoot(page({ page_layouter: { layouter: 'single-pane', root: home } })), home);
	assert.deepEqual(leafPanes(home), ['top', 'left', 'middle']);
	assert.equal(findPane(home, 'left')?.width, '240px');
	assert.equal(findPane(home, 'nope'), undefined);
});

test('pane sizes along the parent axis, with limits', () => {
	assert.equal(paneStyle({ pane_id: 'a', width: '240px', min_width: '160px' }, 'width'), 'flex: 0 0 240px; min-width: 160px; min-height: 50px');
	assert.equal(paneStyle({ pane_id: 'a', width: '240px' }, 'width', 300), 'flex: 0 0 300px; min-width: 50px; min-height: 50px');
	assert.equal(paneStyle({ pane_id: 'a' }, 'width'), 'flex: 1 1 0px; min-width: 50px; min-height: 50px');
	assert.equal(paneStyle({ pane_id: 'a', height: 'auto', max_height: '30%' }, 'height'), 'flex: 0 0 auto; min-width: 50px; min-height: 50px; max-height: 30%');
	assert.equal(paneStyle({ pane_id: 'a', height: '48' }, 'height'), 'flex: 0 0 48px; min-width: 50px; min-height: 50px');
	assert.match(splitStyle({ pane_id: 'r', split: 'columns' }), /flex-direction: row/);
	assert.match(splitStyle({ pane_id: 'r', split: 'rows' }), /flex-direction: column/);
});

test('a leaf pane places what it shows inside its margins', () => {
	const css = placeStyle({ left_margin: '8', top_margin: '1.5 rem', hori_align: 'left', vert_align: 'bottom', hori_scrollbar: 'hide' });
	assert.match(css, /padding: 1.5rem 0px 0px 8px/);
	assert.match(css, /align-items: safe flex-start/);
	assert.match(css, /justify-content: safe flex-end/);
	assert.match(css, /overflow-x: hidden/);
	assert.match(css, /overflow-y: auto/);
	assert.match(placeStyle({}), /padding: 0px 0px 0px 0px; align-items: safe center; justify-content: flex-start/);
});

test('page box: sizes, colors, height rule, host alignment', () => {
	assert.equal(
		pageBoxStyle(page({ width: '50 %', height: '400', min_width: '320', max_height: '600px', bkgd_color: '#fff', frgd_color: 'auto' })),
		'display: flex; flex-direction: column; box-sizing: border-box; flex: none; min-width: 0; width: 50%; height: 400px; min-width: 320px; min-height: 50px; max-height: 600px; background-color: #fff'
	);
	assert.match(pageBoxStyle(page()), /width: 100%/);
	assert.doesNotMatch(pageBoxStyle(page()), /height: 100%/);
	const homePage = page({ page_layouter: { layouter: 'top-left-middle-right-bottom', root: home } });
	assert.equal(fillsHeight(homePage), true, 'rows with an elastic row fill the page area');
	assert.match(pageBoxStyle(homePage), /height: 100%/);
	assert.equal(fillsHeight(page({ page_layouter: { layouter: 'x', root: { pane_id: 'r', split: 'rows', children: [{ pane_id: 'a', height: '40px' }, { pane_id: 'b', height: 'auto' }] } } })), false);
	assert.match(pageBoxStyle(page({ max_width: '640px' }), { pane_id: 'middle', hori_align: 'left' }), /max-width: 640px; align-items: safe flex-start; justify-content: flex-start/);
});

test('only pixel-sized panes are drag-resizable, within min/max', () => {
	assert.equal(isResizable({ pane_id: 'a', width: '240px', resizable: true }, 'width'), true);
	assert.equal(isResizable({ pane_id: 'a', width: 'elastic', resizable: true }, 'width'), false);
	assert.equal(isResizable({ pane_id: 'a', width: '240px' }, 'width'), false);
	assert.equal(isResizable({ pane_id: 'a', height: '48px', resizable: true }, 'width'), false);
	assert.equal(clampPx(100, '160px', '400px'), 160);
	assert.equal(clampPx(500, '160px', '400px'), 400);
	assert.equal(clampPx(250), 250);
});

test('lengths without a unit are pixels', () => {
	assert.equal(cssLength('30'), '30px');
	assert.equal(cssLength('1.5 rem'), '1.5rem');
	assert.equal(cssLength(undefined), undefined);
});

test('split panes place their children inside their margins', () => {
 const css = splitStyle({ pane_id: 'root', split: 'rows', left_margin: '12px', top_margin: '2rem' });
 assert.match(css, /box-sizing: border-box/);
 assert.match(css, /padding: 2rem 0px 0px 12px/);
});

test('pane and panel colors honor pane background precedence and theme pairs', () => {
	assert.equal(surfaceColorStyle({ background_color: { light: '#fff', dark: '#000' }, foreground_color: 'red' }), 'background-color: light-dark(#fff, #000); color: red');
	assert.equal(surfaceColorStyle({ background_color: 'blue', foreground_color: 'white' }, true), 'color: white');
	assert.equal(surfaceColorStyle({}), '');
});
