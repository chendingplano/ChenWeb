export type ElementHandle = 'nw' | 'ne' | 'sw' | 'se' | 'n' | 's' | 'e' | 'w';

export function resizeHandles(width: number, height: number, zoom: number): ElementHandle[] {
 const handles: ElementHandle[] = ['nw', 'ne', 'sw', 'se'];
 if (width * zoom >= 48) handles.push('n', 's');
 if (height * zoom >= 48) handles.push('e', 'w');
 return handles;
}

export function resizeElement(size: { width: number; height: number }, handle: ElementHandle, dx: number, dy: number, zoom: number): { width?: number; height?: number } {
 const result: { width?: number; height?: number } = {};
 if (handle.includes('e') || handle.includes('w')) result.width = Math.max(1, Math.round(size.width + dx / zoom * (handle.includes('w') ? -1 : 1)));
 if (handle.includes('n') || handle.includes('s')) result.height = Math.max(1, Math.round(size.height + dy / zoom * (handle.includes('n') ? -1 : 1)));
 return result;
}

export function resizeOutline(box: { x: number; y: number; width: number; height: number }, handle: ElementHandle, size: { width?: number; height?: number }) {
 const width = size.width ?? box.width, height = size.height ?? box.height;
 return { x: box.x + (handle.includes('w') ? box.width - width : 0), y: box.y + (handle.includes('n') ? box.height - height : 0), width, height };
}

export type ResizeLimit = { source: 'element' | 'panel'; key: string; axis: 'width' | 'height'; kind: 'min' | 'max'; pixels: number };
export type ResizeLimitChange = { source: 'element' | 'panel'; key: string; value: string };
export function resizeLimitChanges(size: { width?: number; height?: number }, limits: ResizeLimit[]): ResizeLimitChange[] {
 return limits.flatMap(({ source, key, axis, kind, pixels }) => {
  const value = size[axis];
  return value !== undefined && (kind === 'min' ? value < pixels : value > pixels) ? [{ source, key, value: `${value}px` }] : [];
 });
}
