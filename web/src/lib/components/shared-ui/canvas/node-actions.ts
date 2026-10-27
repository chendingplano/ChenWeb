// The node tool bar's actions (BaseNode). They change the canvas that holds the
// nodes, so that canvas provides them through context and records each change
// in its undo history.
import { getContext, setContext } from 'svelte';
import type { Edge, Node } from '@xyflow/svelte';

export type NodeActions = {
	duplicate: (id: string) => void;
	remove: (id: string) => void;
	// Opens the class of an element node for editing; only the WPI editor has classes.
	editClass?: (id: string) => void;
};

const KEY = Symbol('canvas-node-actions');

export function setNodeActions(actions: NodeActions) {
	setContext(KEY, actions);
}

export function getNodeActions(): NodeActions | undefined {
	return getContext<NodeActions | undefined>(KEY);
}

// The nodes with a copy of the node, named newId, `step` right of and below it
// (further while another node sits there); the copy becomes the selection. It
// keeps the node's data and size; edges are not copied.
export function duplicateNode(nodes: Node[], id: string, newId: string, step: number): Node[] {
	const src = nodes.find((n) => n.id === id);
	if (!src) return nodes;
	const position = { x: src.position.x + step, y: src.position.y + step };
	while (nodes.some((n) => n.position.x === position.x && n.position.y === position.y)) {
		position.x += step;
		position.y += step;
	}
	const copy: Node = { ...(JSON.parse(JSON.stringify(src)) as Node), id: newId, position, selected: true };
	return [...nodes.map((n) => ({ ...n, selected: false })), copy];
}

// The canvas without the node and the edges that touch it.
export function removeNode(nodes: Node[], edges: Edge[], id: string): { nodes: Node[]; edges: Edge[] } {
	return {
		nodes: nodes.filter((n) => n.id !== id),
		edges: edges.filter((e) => e.source !== id && e.target !== id)
	};
}
