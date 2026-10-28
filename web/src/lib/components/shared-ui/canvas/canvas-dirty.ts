import type { Node, Edge } from '@xyflow/svelte';

// Svelte Flow updates these fields as users select nodes and the canvas measures them.
// They are view state, not edits to the flow.
function content(nodes: Node[], edges: Edge[]) {
	return {
		nodes: nodes.map(({ selected, dragging, measured, ...node }) => node),
		edges: edges.map(({ selected, ...edge }) => edge)
	};
}

export function sameCanvasContent(a: { nodes: Node[]; edges: Edge[] }, b: { nodes: Node[]; edges: Edge[] }): boolean {
	return JSON.stringify(content(a.nodes, a.edges)) === JSON.stringify(content(b.nodes, b.edges));
}
