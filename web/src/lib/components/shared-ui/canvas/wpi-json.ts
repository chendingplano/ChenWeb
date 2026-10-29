// The WPI descriptors the editor's canvas stands for, shown by View JSON and
// edited in the properties panel's Node tab: an element node as the element
// instance a page holds, and the canvas as a page descriptor (devdoc 2026102601
// §3.2.1, §3.5).
import type { Node } from '@xyflow/svelte';
import type { JsonObject } from '$lib/wpi/types';
import { withListSourceDefaults } from './element-attributes';

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
	const element = String(node.data.element ?? '');
	const attrs = (node.data.attrs as JsonObject | undefined) ?? {};
	return { element, ...(element === 'list' ? withListSourceDefaults(attrs, undefined) : attrs) };
}

// The node's data after its JSON was edited, the inverse of nodeJson. An element
// node keeps the rest of its data, such as its name in data.label.
export function nodeData(node: Node, json: JsonObject): Record<string, unknown> {
	if (!isElementNode(node)) return json;
	const { element, ...attrs } = json;
	return { ...node.data, element, attrs };
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

// Import every instance, including repeated classes, retaining all its attributes.
export function pageNodes(page: JsonObject): Node[] {
	if (!Array.isArray(page.elements)) throw new Error('Invalid page elements');
	return page.elements.map((instance, index) => {
		if (
			!instance ||
			typeof instance !== 'object' ||
			Array.isArray(instance) ||
			typeof instance.element !== 'string'
		) {
			throw new Error('Invalid element instance');
		}
		const { element, ...attrs } = instance;
		return {
			id: `wpi-element-loaded-${index}`,
			type: 'wpi-element',
			position: { x: 60 + (index % 4) * 240, y: 60 + Math.floor(index / 4) * 160 },
			data: { label: element, element, attrs }
		};
	});
}

// The editor can apply JSON objects as page or node descriptors.
export function parseJsonObject(text: string): JsonObject {
	const value: unknown = JSON.parse(text);
	if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Expected an object');
	return value as JsonObject;
}
