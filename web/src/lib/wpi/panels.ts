// Pane and panel bookkeeping (devdoc 2026102601 §3.3, §10). Pure functions
// over plain data, so the runtime can keep the data in Svelte $state.
import { canonical } from './dirty';
import type { InputSpec } from './types';

export type PaneModel = {
	// Panel IDs, least recently shown first; the last one is shown when the
	// visible panel closes.
	panels: string[];
	visible: string | null;
};

export type PanelModel = { id: string; paneId: string; pageId: string; key: string };

// "menuInstance/itemId" → panel ID.
export type Bindings = Record<string, string>;

// Panels opened for the same page with the same input share this key.
export function sharingKey(pageId: string, input: InputSpec | undefined): string {
	return `${pageId}|${canonical(input ?? {})}`;
}

export function isBound(bindings: Bindings, panelId: string): boolean {
	return Object.values(bindings).includes(panelId);
}

export type OpenPlan = {
	// Show this existing panel instead of creating one.
	reuse: string | null;
	// What happens to the panel the pane shows now, if any.
	current: { id: string; fate: 'hide' | 'close' } | null;
};

// Decides how to open a page in a pane. A menu click (binding set) reuses the
// item's panel, or a bound panel of the same page and input; otherwise a new
// panel is created. The panel being replaced is hidden if a menu item is bound
// to it and closed otherwise.
export function planOpen(
	pane: PaneModel,
	panels: Record<string, PanelModel>,
	bindings: Bindings,
	req: { key: string; binding?: string }
): OpenPlan {
	let reuse: string | null = null;
	if (req.binding) {
		const own = bindings[req.binding];
		if (own && pane.panels.includes(own)) {
			reuse = own;
		} else {
			reuse = pane.panels.find((id) => panels[id]?.key === req.key && isBound(bindings, id)) ?? null;
		}
	}
	const visible = pane.visible;
	if (!visible || visible === reuse) return { reuse, current: null };
	return { reuse, current: { id: visible, fate: isBound(bindings, visible) ? 'hide' : 'close' } };
}

// Makes id the visible panel and the most recently shown one.
export function showPanel(pane: PaneModel, id: string): void {
	pane.panels = [...pane.panels.filter((p) => p !== id), id];
	pane.visible = id;
}

// Removes a panel and its bindings. If it was visible, the most recently
// shown remaining panel becomes visible; that panel's ID is returned.
export function removePanel(pane: PaneModel, bindings: Bindings, id: string): string | null {
	pane.panels = pane.panels.filter((p) => p !== id);
	for (const [binding, panelId] of Object.entries(bindings)) {
		if (panelId === id) delete bindings[binding];
	}
	if (pane.visible !== id) return null;
	pane.visible = pane.panels.at(-1) ?? null;
	return pane.visible;
}
