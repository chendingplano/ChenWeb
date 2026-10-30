// Editing a page's pane tree in the WPI editor (OpenSpec wpi-pages-own-panes,
// design D12). The page's own root replaces its page layouter copy's, so every
// edit writes page_layouter.root. Element nodes follow their pane: a split moves
// them to the first child, a rename renames their pane, a delete removes them.
import type { Node } from '@xyflow/svelte';
import { effectiveRoot, findPane, isLeaf, leafPanes } from '$lib/wpi/layout';
import type { JsonObject, PageDescriptor, PaneSpec } from '$lib/wpi/types';
import { isElementNode, nodePane, paneNodes } from './wpi-json';

export const PANE_ID_PATTERN = /^[a-z0-9][a-z0-9-]{0,63}$/;

// Attributes that concern what a pane shows; they apply to leaf panes only.
export const LEAF_PANE_ATTRS = ['left_margin', 'right_margin', 'top_margin', 'bottom_margin', 'hori_align', 'vert_align', 'hori_scrollbar', 'vert_scrollbar'];
export const PANE_ATTRS = ['width', 'height', 'min_width', 'min_height', 'max_width', 'max_height', 'resizable'];

export type Edit = { page: JsonObject; nodes: Node[] };

export function rootOf(page: JsonObject): PaneSpec {
	return effectiveRoot(page as unknown as PageDescriptor);
}

function withRoot(page: JsonObject, root: PaneSpec): JsonObject {
	const pl = (page.page_layouter ?? { layouter: 'single-pane' }) as JsonObject;
	return { ...page, page_layouter: { ...pl, root: root as unknown as JsonObject } };
}

// Apply the selected class tree, retaining matching panels and moving displaced
// elements to its first leaf so they remain visible and can be repositioned.
export function setPageLayouter({ page, nodes }: Edit, cls: JsonObject): Edit {
 const next: JsonObject = { ...page, page_layouter: { layouter: cls.descriptor_id, class: cls } };
 const leaves = leafPanes(rootOf(next));
 const panels = panelsOf(page);
 const kept: JsonObject = {};
 for (const id of leaves) if (panels[id]) kept[id] = panels[id];
 if (!kept[leaves[0]]) {
  const displaced = Object.keys(panels).find((id) => !leaves.includes(id));
  if (displaced) kept[leaves[0]] = panels[displaced];
 }
 return {
  page: { ...next, panels: kept },
  nodes: nodes.map((n) => isElementNode(n) && !leaves.includes(nodePane(n)) ? { ...n, data: { ...n.data, pane: leaves[0] } } : n)
 };
}

// The tree with pane id replaced by what fn returns for it.
function mapPane(pane: PaneSpec, id: string, fn: (p: PaneSpec) => PaneSpec): PaneSpec {
	if (pane.pane_id === id) return fn(pane);
	if (!pane.children) return pane;
	return { ...pane, children: pane.children.map((c) => mapPane(c, id, fn)) };
}

function panelsOf(page: JsonObject): JsonObject {
	return (page.panels && typeof page.panels === 'object' && !Array.isArray(page.panels) ? page.panels : {}) as JsonObject;
}

function allIds(pane: PaneSpec): string[] {
	return [pane.pane_id, ...(pane.children ?? []).flatMap(allIds)];
}

// The first ID from base that is not used: base, base-2, base-3, ...
function freeId(used: Set<string>, base: string): string {
	if (!used.has(base)) return base;
	for (let i = 2; ; i++) if (!used.has(`${base}-${i}`)) return `${base}-${i}`;
}

export function freePaneId(page: JsonObject, base: string): string {
	return freeId(new Set(allIds(rootOf(page))), base);
}

// Sets or clears one attribute of a pane; text "" clears it.
export function setPaneAttr(page: JsonObject, id: string, name: string, value: unknown): JsonObject {
	return withRoot(page, mapPane(rootOf(page), id, (p) => {
		const next = { ...p } as Record<string, unknown>;
		if (value === undefined || value === '') delete next[name];
		else next[name] = value;
		return next as PaneSpec;
	}));
}

// Splits leaf pane id into rows or columns: it gets two children; the first
// takes its panel, elements and leaf-only attributes.
export function splitPane({ page, nodes }: Edit, id: string, split: 'rows' | 'columns'): Edit {
	const pane = findPane(rootOf(page), id);
	if (!pane || !isLeaf(pane)) return { page, nodes };
	const used = new Set(allIds(rootOf(page)));
	const first = freeId(used, `${id}-1`);
	const second = freeId(used.add(first), `${id}-2`);
	const leafAttrs: Record<string, unknown> = {};
	const rest: Record<string, unknown> = {};
	for (const [k, v] of Object.entries(pane)) (LEAF_PANE_ATTRS.includes(k) ? leafAttrs : rest)[k] = v;
	const next = withRoot(page, mapPane(rootOf(page), id, () => ({ ...(rest as PaneSpec), split, children: [{ pane_id: first, ...leafAttrs }, { pane_id: second }] })));
	const panels = { ...panelsOf(page) };
	if (panels[id]) {
		panels[first] = panels[id];
		delete panels[id];
	}
	return { page: { ...next, panels }, nodes: nodes.map((n) => (isElementNode(n) && nodePane(n) === id ? { ...n, data: { ...n.data, pane: first } } : n)) };
}

// Adds a leaf pane at the end of split pane id.
export function addPane(page: JsonObject, id: string): JsonObject {
	const pane = findPane(rootOf(page), id);
	if (!pane || isLeaf(pane)) return page;
	const child = freePaneId(page, `${id}-${(pane.children?.length ?? 0) + 1}`);
	return withRoot(page, mapPane(rootOf(page), id, (p) => ({ ...p, children: [...(p.children ?? []), { pane_id: child }] })));
}

// The element nodes a delete of pane id removes: those of its leaf panes.
export function nodesUnder(page: JsonObject, nodes: Node[], id: string): Node[] {
	const pane = findPane(rootOf(page), id);
	return pane ? leafPanes(pane).flatMap((leaf) => paneNodes(nodes, leaf)) : [];
}

// Deletes pane id, not the root, with its panes, panels and element nodes. A
// split pane left with one child is replaced by it, keeping its own sizes.
export function deletePane({ page, nodes }: Edit, id: string): Edit {
	const root = rootOf(page);
	const pane = findPane(root, id);
	if (!pane || id === root.pane_id) return { page, nodes };
	const gone = new Set(allIds(pane));
	const removeFrom = (p: PaneSpec): PaneSpec => {
		if (!p.children) return p;
		const children = p.children.filter((c) => c.pane_id !== id).map(removeFrom);
		if (children.length === 1 && p.children.some((c) => c.pane_id === id)) {
			const own = Object.entries(children[0]).filter(([k]) => !PANE_ATTRS.includes(k));
			const sizes = Object.entries(p).filter(([k]) => PANE_ATTRS.includes(k));
			return Object.fromEntries([...own, ...sizes]) as PaneSpec;
		}
		return { ...p, children };
	};
	const panels = { ...panelsOf(page) };
	for (const g of gone) delete panels[g];
	return {
		page: { ...withRoot(page, removeFrom(root)), panels },
		nodes: nodes.filter((n) => !(isElementNode(n) && gone.has(nodePane(n))))
	};
}

export type RenameProblem = 'invalid' | 'taken';

export function renameProblem(page: JsonObject, id: string, to: string): RenameProblem | null {
	if (!PANE_ID_PATTERN.test(to)) return 'invalid';
	if (to !== id && allIds(rootOf(page)).includes(to)) return 'taken';
	return null;
}

// Renames pane id; its panel key and element nodes follow.
export function renamePane({ page, nodes }: Edit, id: string, to: string): Edit {
	if (renameProblem(page, id, to) || to === id) return { page, nodes };
	const panels = { ...panelsOf(page) };
	if (panels[id]) {
		panels[to] = panels[id];
		delete panels[id];
	}
	return {
		page: { ...withRoot(page, mapPane(rootOf(page), id, (p) => ({ ...p, pane_id: to }))), panels },
		nodes: nodes.map((n) => (isElementNode(n) && nodePane(n) === id ? { ...n, data: { ...n.data, pane: to } } : n))
	};
}

// Moves an element node to the end of another leaf pane's panel.
export function moveNode(nodes: Node[], nodeId: string, pane: string): Node[] {
	const last = paneNodes(nodes, pane).filter((n) => n.id !== nodeId).at(-1);
	return nodes.map((n) =>
		n.id === nodeId ? { ...n, position: { x: last ? last.position.x + 1 : 0, y: last ? last.position.y : 0 }, data: { ...n.data, pane } } : n
	);
}

// Sets a panel's element layouter parameters; an empty config clears the
// panel's own, so the copy's apply.
export function setPanelConfig(page: JsonObject, pane: string, config: JsonObject): JsonObject {
	const panels = { ...panelsOf(page) };
	const panel = (panels[pane] ?? { elem_layouter: { layouter: 'flow-layouter' } }) as JsonObject;
	const layouter = { ...(panel.elem_layouter as JsonObject) };
	if (Object.keys(config).length) layouter.config = config;
	else delete layouter.config;
	panels[pane] = { ...panel, elem_layouter: layouter };
	return { ...page, panels };
}
