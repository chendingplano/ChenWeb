const KEY = 'wpi-editor-component-tree';
export type TreeLayout = { open: boolean; x: number; y: number; docked?: boolean };

export function readTreeLayout(storage: Pick<Storage, 'getItem'>): TreeLayout | null {
 try {
  const value = JSON.parse(storage.getItem(KEY) ?? 'null');
  return value && typeof value.open === 'boolean' && Number.isFinite(value.x) && Number.isFinite(value.y) ? { ...value, docked: value.docked === true } : null;
 } catch { return null; }
}

export function writeTreeLayout(storage: Pick<Storage, 'setItem'>, layout: TreeLayout) {
 try { storage.setItem(KEY, JSON.stringify(layout)); } catch { /* Storage is optional. */ }
}
