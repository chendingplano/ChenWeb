export type ElementHandle = 'nw' | 'ne' | 'sw' | 'se' | 'n' | 's' | 'e' | 'w';

export function resizeHandles(width: number, height: number, zoom: number): ElementHandle[] {
 const handles: ElementHandle[] = ['nw', 'ne', 'sw', 'se'];
 if (width * zoom >= 48) handles.push('n', 's');
 if (height * zoom >= 48) handles.push('e', 'w');
 return handles;
}

export function resizeElement(size: { width: number; height: number }, handle: ElementHandle, dx: number, dy: number, zoom: number): { width?: number; height?: number } {
 const result: { width?: number; height?: number } = {};
 if (handle.includes('e') || handle.includes('w')) result.width = Math.max(20, Math.round(size.width + dx / zoom * (handle.includes('w') ? -1 : 1)));
 if (handle.includes('n') || handle.includes('s')) result.height = Math.max(20, Math.round(size.height + dy / zoom * (handle.includes('n') ? -1 : 1)));
 return result;
}
