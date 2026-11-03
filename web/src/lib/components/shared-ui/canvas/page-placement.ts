type Size = { width: number; height: number };
type Point = { x: number; y: number };

// The canvas size of a new page. blankPage() writes it into the descriptor too,
// so the page config shows the size the canvas is drawn at.
export const DEFAULT_PREVIEW_SIZE = { width: 1000, height: 650 };

// A new page: one pane, main, whose panel uses the flow layouter. Saving it
// gives it copies of the active classes (OpenSpec wpi-pages-own-panes, D4).
export function blankPage() {
 return {
  descriptor_id: '',
  schema_version: '0.5',
  width: `${DEFAULT_PREVIEW_SIZE.width}px`,
  height: `${DEFAULT_PREVIEW_SIZE.height}px`,
  page_layouter: { layouter: 'single-pane', root: { pane_id: 'main' } },
  panels: { main: { elem_layouter: { layouter: 'flow-layouter' }, elem_list: [] } }
 };
}

export function isInsidePage(point: Point, page: Size): boolean {
 return point.x >= 0 && point.y >= 0 && point.x < page.width && point.y < page.height;
}

export function centeredPageViewport(page: Size, canvas: Size) {
 const zoom = Math.max(0.1, Math.min(1, (canvas.width - 80) / page.width, (canvas.height - 80) / page.height));
 return { x: (canvas.width - page.width * zoom) / 2, y: (canvas.height - page.height * zoom) / 2, zoom };
}
