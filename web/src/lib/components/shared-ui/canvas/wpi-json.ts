// The WPI descriptors the editor's canvas stands for, shown by View JSON: an
// element node as the element instance a page holds, and the canvas as a page
// descriptor (devdoc 2026102601 §3.2.1, §3.5).
import type { Node } from '@xyflow/svelte';
import type { JsonObject } from '$lib/wpi/types';

// Pages are format 0.2, and flow-layouter is the only layouter.
const PAGE_SCHEMA_VERSION = '0.2';
const PAGE_LAYOUTER = 'flow-layouter';

export function isElementNode(node: Node): boolean {
	return node.type === 'wpi-element';
}

// An element node as an instance: its class, then the attributes the author set.
// Other nodes have no WPI form yet, so their data is shown as it is.
export function nodeJson(node: Node): JsonObject {
	if (!isElementNode(node)) return node.data as JsonObject;
	return { element: String(node.data.element ?? ''), ...((node.data.attrs as JsonObject | undefined) ?? {}) };
}

// The canvas as a page: its element nodes in reading order (top to bottom, then
// left to right), the order flow-layouter places them in. The canvas has no page
// ID, so descriptor_id is left out.
export function pageJson(nodes: Node[]): JsonObject {
	const elements = nodes
		.filter(isElementNode)
		.sort((a, b) => a.position.y - b.position.y || a.position.x - b.position.x)
		.map(nodeJson);
	return { schema_version: PAGE_SCHEMA_VERSION, layouter: PAGE_LAYOUTER, elements };
}
