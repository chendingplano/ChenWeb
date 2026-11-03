import { expect, test } from 'bun:test';
import { isInsidePage, centeredPageViewport, blankPage } from './page-placement';

test('blank page has no persisted identifier, starts empty, and carries the canvas size', () => {
 expect(blankPage()).toEqual({ descriptor_id: '', schema_version: '0.5', width: '1000px', height: '650px', page_layouter: { layouter: 'single-pane', root: { pane_id: 'main' } }, panels: { main: { elem_layouter: { layouter: 'flow-layouter' }, elem_list: [] } } });
});
test('element drops must be within the page bounds', () => {
 const page = { width: 1000, height: 650 };
 expect(isInsidePage({ x: 0, y: 0 }, page)).toBe(true);
 expect(isInsidePage({ x: 999, y: 649 }, page)).toBe(true);
 expect(isInsidePage({ x: -1, y: 10 }, page)).toBe(false);
 expect(isInsidePage({ x: 1000, y: 10 }, page)).toBe(false);
});
test('restore centers and fits the page', () => {
 expect(centeredPageViewport({ width: 1000, height: 650 }, { width: 1200, height: 850 })).toEqual({ x: 100, y: 100, zoom: 1 });
 expect(centeredPageViewport({ width: 1000, height: 650 }, { width: 580, height: 405 })).toEqual({ x: 40, y: 40, zoom: 0.5 });
});
