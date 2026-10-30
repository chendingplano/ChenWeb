// The Panel Cache (devdoc 2026102601 §3.3.3; OpenSpec wpi-pages-own-panes,
// design D8): the page instances opened in each pane, as (page-instance-id,
// pane-instance-id) entries. A pane shows one of them, or its own panel when
// it shows none. Switching hides; only closing removes. Pure functions over
// plain data, so the runtime can keep the data in Svelte $state.
import { canonical } from './dirty';
import type { InputSpec } from './types';

export type PaneModel = {
	// Page instances opened in the pane, least recently shown first.
	opened: string[];
	// The page instance shown, or null for the pane's own panel.
	visible: string | null;
};

// Page instances of the same page with the same input share this key: opening
// it again in the same pane shows the cached instance.
export function sharingKey(pageId: string, input: InputSpec | undefined): string {
	return `${pageId}|${canonical(input ?? {})}`;
}

// The cached page instance with key in the pane, if any.
export function findCached(pane: PaneModel, keys: Record<string, { key: string } | undefined>, key: string): string | null {
	return pane.opened.find((id) => keys[id]?.key === key) ?? null;
}

// Makes id the shown page instance and the most recently shown one; the one
// shown before is hidden, not closed.
export function show(pane: PaneModel, id: string): void {
	pane.opened = [...pane.opened.filter((p) => p !== id), id];
	pane.visible = id;
}

// Removes a closed page instance's entry. A pane that showed it shows its own
// panel again.
export function forget(pane: PaneModel, id: string): void {
	pane.opened = pane.opened.filter((p) => p !== id);
	if (pane.visible === id) pane.visible = null;
}

// A pane instance ID: the page instance that owns the pane, and the pane's ID.
export function paneInstanceId(pageInstanceId: string, paneId: string): string {
	return `${pageInstanceId}/${paneId}`;
}

// The page instance that owns a pane instance. Page and pane IDs have no "/".
export function paneOwner(paneInstance: string): string {
	return paneInstance.slice(0, paneInstance.lastIndexOf('/'));
}
