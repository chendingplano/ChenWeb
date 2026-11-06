import { expect, test } from 'bun:test';
import { readTreeLayout, writeTreeLayout } from './wpi-tree-layout';

test('restores visibility and the last position, including after closing', () => {
 let value = '';
 const storage = { getItem: () => value, setItem: (_key: string, data: string) => { value = data; } };
 writeTreeLayout(storage, { open: true, x: 320, y: 180 });
 expect(readTreeLayout(storage)).toEqual({ open: true, x: 320, y: 180, docked: false });
 writeTreeLayout(storage, { open: false, x: 320, y: 180 });
 expect(readTreeLayout(storage)).toEqual({ open: false, x: 320, y: 180, docked: false });
});

test('ignores invalid or unavailable storage', () => {
 expect(readTreeLayout({ getItem: () => '{bad' })).toBeNull();
 expect(readTreeLayout({ getItem: () => '{"open":true,"x":null,"y":4}' })).toBeNull();
 expect(readTreeLayout({ getItem: () => { throw new Error(); } })).toBeNull();
});

test('restores the docked state and treats older layouts as floating', () => {
 expect(readTreeLayout({ getItem: () => '{"open":true,"x":1,"y":2,"docked":true}' })?.docked).toBe(true);
 expect(readTreeLayout({ getItem: () => '{"open":true,"x":1,"y":2}' })?.docked).toBe(false);
});
