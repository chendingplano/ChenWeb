import { expect, test } from 'bun:test';
import { pageMinimapBounds } from './page-minimap';

test('overview includes the page and the current viewport after pan and zoom', () => {
 const result = pageMinimapBounds({ width: 800, height: 600 }, { width: 1000, height: 500 }, { x: -600, y: -100, zoom: 2 });
 expect(result.viewport).toEqual({ x: 300, y: 50, width: 500, height: 250 });
 expect(result.bounds).toEqual({ x: 0, y: 0, width: 800, height: 600 });
 const moved = pageMinimapBounds({ width: 800, height: 600 }, { width: 1000, height: 500 }, { x: -1800, y: 200, zoom: 2 });
 expect(moved.viewport).toEqual({ x: 900, y: -100, width: 500, height: 250 });
 expect(moved.bounds).toEqual({ x: 0, y: -100, width: 1400, height: 700 });
});
