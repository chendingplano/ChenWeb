// The WPI descriptors the editor's canvas stands for, shown by View JSON and
// edited in the properties panel's Node tab: an element node as the element
// instance a panel holds, and the canvas as a page descriptor whose panels hold
// the element nodes of their pane (devdoc 2026102601 §3.2, §3.3, §3.5; OpenSpec
// wpi-pages-own-panes, design D12). An element node's data is {label, element,
// attrs, class?, pane}: class is the copy of its class the instance holds, kept
// out of the node's JSON and put back in the page.
import type { Node } from '@xyflow/svelte';
import { effectiveRoot, leafPanes } from '$lib/wpi/layout';
import { newDynamicInstanceId, stampInstances, stripDynamicIds } from '$lib/wpi/instance-ids';
import type { Json, JsonObject, PageDescriptor, PaneSpec } from '$lib/wpi/types';
import { normalizeElementLengths, withListSourceDefaults } from './element-attributes';

export const PAGE_SCHEMA_VERSION = '0.5';

export function isElementNode(node: Node): boolean {
	return node.type === 'wpi-element';
}

// An element node as an instance: its class, then the attributes the author set.
// Other nodes have no WPI form yet, so their data is shown as it is.
export function nodeJson(node: Node): JsonObject {
	if (!isElementNode(node)) return node.data as JsonObject;
	const element = String(node.data.element ?? '');
	const attrs = (node.data.attrs as JsonObject | undefined) ?? {};
	return { element, ...normalizeElementLengths(element === 'list' ? withListSourceDefaults(attrs, undefined) : attrs) };
}

// An element node's instance_id is saved with the page; its dynamic_instance_id is
// generated at run time and shown here only (devdoc 2026102601 §3.4).
export function nodeViewJson(node: Node): JsonObject {
 if (!isElementNode(node)) return nodeJson(node);
 const { instance_id, dynamic_instance_id } = node.data as { instance_id?: string; dynamic_instance_id?: string };
 return { ...(instance_id ? { instance_id } : {}), ...(dynamic_instance_id ? { dynamic_instance_id } : {}), ...nodeJson(node) };
}

export function elementIssueLocation(path: string, nodes: Node[], page: JsonObject): { pane: string; index: number; id: string; field: string } | null {
 const match = path.match(/^\/panels\/([^/]+)\/elem_list\/(\d+)(?:\/(.*))?$/);
 if (!match) return null;
 const pane = match[1].replace(/~1/g, '/').replace(/~0/g, '~');
 const leaves = leafPanes(effectiveRoot(page as unknown as PageDescriptor));
 const placed = nodes.filter(isElementNode).map(n => leaves.includes(nodePane(n)) ? n : { ...n, data: { ...n.data, pane: leaves[0] } });
 const index = Number(match[2]);
 const node = paneNodes(placed, pane)[index];
 return node ? { pane, index, id: node.id, field: match[3] ?? '' } : null;
}

// The node's data after its JSON was edited, the inverse of nodeJson. An element
// node keeps the rest of its data: its name in data.label, its class copy and
// its pane.
export function nodeData(node: Node, json: JsonObject): Record<string, unknown> {
	if (!isElementNode(node)) return json;
	const { element, class: _copy, instance_id: _id, dynamic_instance_id: _dyn, ...attrs } = json;
	return { ...node.data, element, attrs };
}

// The pane an element node is in.
export function nodePane(node: Node): string {
	return String(node.data.pane ?? '');
}

// The element nodes of a pane in reading order (top to bottom, then left to
// right), the order flow-layouter places them in.
export function paneNodes(nodes: Node[], pane: string): Node[] {
	return nodes
		.filter((n) => isElementNode(n) && nodePane(n) === pane)
		.sort((a, b) => a.position.y - b.position.y || a.position.x - b.position.x);
}

// An element node as the instance its panel stores: with its instance_id and class
// copy, and without any dynamic_instance_id, its own or its validators' and actions'.
export function instanceJson(node: Node): JsonObject {
	const copy = node.data.class as JsonObject | undefined;
	const id = node.data.instance_id as string | undefined;
	const { element, ...rest } = stripDynamicIds(nodeJson(node));
	return { element, ...(id ? { instance_id: id } : {}), ...rest, ...(copy ? { class: copy } : {}) };
}

// The canvas as a page: the page's attributes and page layouter, and for each
// leaf pane a panel whose elem_list is the pane's element nodes. A leaf pane
// with element nodes but no panel gets one with a flow-layouter. Element nodes
// of a pane that is not a leaf go to the first leaf.
export function pageJson(nodes: Node[], page: JsonObject): JsonObject {
	const leaves = leafPanes(effectiveRoot(page as unknown as PageDescriptor));
	const placed = nodes.filter(isElementNode).map((n) => (leaves.includes(nodePane(n)) ? n : { ...n, data: { ...n.data, pane: leaves[0] } }));
	const before = (page.panels && typeof page.panels === 'object' && !Array.isArray(page.panels) ? page.panels : {}) as JsonObject;
	const panels: JsonObject = {};
	for (const pane of leaves) {
		const own = paneNodes(placed, pane);
		const panel = before[pane] as JsonObject | undefined;
		if (!panel && own.length === 0) continue;
		panels[pane] = { ...(panel ?? { elem_layouter: { layouter: 'flow-layouter' } }), elem_list: own.map(instanceJson) };
	}
	return stripDynamicIds({ ...page, schema_version: PAGE_SCHEMA_VERSION, panels });
}

// One editable node per element instance of every panel, in the order of the
// leaf panes, keeping repeated classes, class copies and all attributes. Nodes
// are laid out in a grid per pane, so reading order is the panel's order.
export function pageNodes(page: JsonObject): Node[] {
	const leaves = leafPanes(effectiveRoot(page as unknown as PageDescriptor));
	const panels = (page.panels ?? {}) as Record<string, { elem_list?: Json[] }>;
	if (typeof panels !== 'object' || Array.isArray(panels)) throw new Error('Invalid page panels');
	const nodes: Node[] = [];
	leaves.forEach((pane, p) => {
		const list = panels[pane]?.elem_list ?? [];
		if (!Array.isArray(list)) throw new Error('Invalid panel elements');
		list.forEach((instance, index) => {
			if (!instance || typeof instance !== 'object' || Array.isArray(instance) || typeof instance.element !== 'string') {
				throw new Error('Invalid element instance');
			}
			const { element, class: copy, instance_id, ...attrs } = instance;
			nodes.push({
				id: `wpi-element-loaded-${pane}-${index}`,
				type: 'wpi-element',
				position: { x: 60 + (index % 4) * 240, y: 60 + p * 4000 + Math.floor(index / 4) * 160 },
				data: { label: element, element, attrs: stampInstances(attrs, null), pane, dynamic_instance_id: newDynamicInstanceId(), ...(typeof instance_id === 'string' ? { instance_id } : {}), ...(copy ? { class: copy } : {}) }
			});
		});
	});
	return nodes;
}

// The editor can apply JSON objects as page or node descriptors.
export function parseJsonObject(text: string): JsonObject {
	const value: unknown = JSON.parse(text);
	if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Expected an object');
	return value as JsonObject;
}

// The same pane assignment and reading order as the page descriptor.
export function elementTree(nodes: Node[], page: JsonObject, labels: { pane: string; panel: string; layouter: string; elements: string }): string {
 const root = effectiveRoot(page as unknown as PageDescriptor);
 const assembled = pageJson(nodes, page);
 const panels = assembled.panels as Record<string, { elem_layouter?: { layouter?: string }; elem_list?: JsonObject[] }>;
 function lines(pane: PaneSpec, depth: number): string[] {
  const indent = '  '.repeat(depth);
  const title = indent + pane.pane_id + ' ' + labels.pane;
  if (pane.split) return [title, ...(pane.children ?? []).flatMap(child => lines(child, depth + 1))];
  return [title, indent + '  ' + labels.panel, indent + '    ' + labels.layouter + ': ' + (panels[pane.pane_id]?.elem_layouter?.layouter ?? ''), indent + '    ' + labels.elements, ...(panels[pane.pane_id]?.elem_list ?? []).map(element => indent + '      ' + String(element.element ?? ''))];
 }
 return lines(root, 0).join('\n');
}

// What a component-tree node selects in the editor: an element's canvas node, or
// the Properties panel's view of the app, the page, a pane or a panel.
export type ComponentTarget =
 | { kind: 'app' } | { kind: 'page' } | { kind: 'pane'; pane: string } | { kind: 'panel'; pane: string } | { kind: 'element'; node: string };
export type ComponentNode = { id: string; label: string; target?: ComponentTarget; children: ComponentNode[] };

// The page as a component tree: App, the page with its page layouter (whose panes
// nest as in the page layouter), its element layouters per leaf pane, and its
// elements in pane order. Elements hold no nested elements yet, so they are leaves.
export function componentTree(nodes: Node[], page: JsonObject, labels: { app: string; page: string; pageLayouter: string; elementLayouter: string; elements: string }): ComponentNode {
 const root = effectiveRoot(page as unknown as PageDescriptor);
 const leaves = leafPanes(root);
 const placed = nodes.filter(isElementNode).map((n) => (leaves.includes(nodePane(n)) ? n : { ...n, data: { ...n.data, pane: leaves[0] } }));
 const panels = pageJson(nodes, page).panels as Record<string, { elem_layouter?: { layouter?: string } }>;
 function pane(spec: PaneSpec): ComponentNode {
  return { id: 'pane:' + spec.pane_id, label: spec.split ? spec.pane_id + ' (' + spec.split + ')' : spec.pane_id, target: { kind: 'pane', pane: spec.pane_id }, children: (spec.children ?? []).map(pane) };
 }
 return {
  id: 'app', label: labels.app, target: { kind: 'app' }, children: [{
   id: 'page', label: labels.page, target: { kind: 'page' }, children: [
    { id: 'page-layouter', label: labels.pageLayouter, target: { kind: 'pane', pane: root.pane_id }, children: [pane(root)] },
    { id: 'element-layouter', label: labels.elementLayouter, target: { kind: 'panel', pane: leaves[0] }, children: leaves.map(id => {
     const layouter = panels[id]?.elem_layouter?.layouter;
     return { id: 'element-layouter:' + id, label: layouter ? id + ': ' + layouter : id, target: { kind: 'panel', pane: id }, children: [] };
    }) },
    { id: 'elements', label: labels.elements, children: leaves.flatMap(id => paneNodes(placed, id).map((node, index) => ({
     id: 'element:' + id + ':' + index, label: 'element:' + String(node.data.element ?? ''), target: { kind: 'element', node: node.id }, children: [],
    }))) },
   ],
  }],
 };
}
