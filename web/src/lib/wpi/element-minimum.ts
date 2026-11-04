/** Minimum space for the color editor's heading and both color rows, in CSS pixels. */
export function elementMinimum(module: string, editor: unknown): { width: number; height: number } | undefined {
 return module === 'value-editor' && editor === 'color' ? { width: 280, height: 120 } : undefined;
}
export function belowElementMinimum(size: { width?: number; height?: number }, minimum?: { width: number; height: number }): boolean {
 return !!minimum && ((size.width !== undefined && size.width < minimum.width) || (size.height !== undefined && size.height < minimum.height));
}
