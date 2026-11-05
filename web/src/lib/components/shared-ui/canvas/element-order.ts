// Reordering the element nodes of a pane by dragging one of them (the flow layouter
// places them in reading order, which pageJson takes from the nodes' positions).
import type { Node } from '@xyflow/svelte';
import { paneNodes } from './wpi-json';

export type Box = { left: number; top: number; right: number; bottom: number };

// The index, among the other elements of a pane in reading order, where a drop at
// (x, y) lands: before the first element that the pointer is above, or left of in
// its row.
export function dropIndex(boxes: Box[], x: number, y: number): number {
	const index = boxes.findIndex((b) => y < b.bottom && (y < b.top || x < (b.left + b.right) / 2));
	return index === -1 ? boxes.length : index;
}

// Moves an element node to `index` among the others of its pane. Each node keeps
// one of the pane's existing positions, so the layout's slots stay where they were
// and only which element sits in which slot changes.
export function moveElementNode(nodes: Node[], id: string, index: number): Node[] {
	const moved = nodes.find((n) => n.id === id);
	if (!moved) return nodes;
	const own = paneNodes(nodes, String(moved.data.pane ?? ''));
	const slots = own.map((n) => ({ ...n.position }));
	const order = own.filter((n) => n.id !== id);
	order.splice(index, 0, moved);
	const positions = new Map(order.map((n, i) => [n.id, slots[i]]));
	return nodes.map((n) => (positions.has(n.id) ? { ...n, position: { ...positions.get(n.id)! } } : n));
}
