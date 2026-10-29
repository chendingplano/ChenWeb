type Size = { width: number; height: number };
type Viewport = { x: number; y: number; zoom: number };

export function pageMinimapBounds(page: Size, canvas: Size, transform: Viewport) {
 const viewport = { x: -transform.x / transform.zoom, y: -transform.y / transform.zoom, width: canvas.width / transform.zoom, height: canvas.height / transform.zoom };
 const x = Math.min(0, viewport.x);
 const y = Math.min(0, viewport.y);
 return { viewport, bounds: { x, y, width: Math.max(page.width, viewport.x + viewport.width) - x, height: Math.max(page.height, viewport.y + viewport.height) - y } };
}
