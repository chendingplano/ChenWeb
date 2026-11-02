// What the property editor reads and writes for each kind of entity (OpenSpec
// wpi-property-editors, design D2). Each source reads the live state it is
// built from and reports a whole new value of that state through onChange.
import type { ElementClassSummary } from '../api';
import { localized } from '../text';
import type { Json, JsonObject } from '../types';
import { findPane, isLeaf } from '../layout';
import { flowDefaults, type FlowField } from '$lib/components/shared-ui/canvas/flow-config';
import { rootOf, setPaneAttr, setPanelAttr, setPanelConfig } from '$lib/components/shared-ui/canvas/pane-edit';
import type { PropertySource } from './types';

function without(obj: JsonObject, name: string): JsonObject {
	const next = { ...obj };
	delete next[name];
	return next;
}

const none = { fixed: () => false, required: () => false, helpTip: () => '', defaultValue: () => undefined };

// An element instance: its own values in attrs, then its class's defaults, then
// the code module's fallbacks.
export function elementSource(
	attrs: JsonObject,
	cls: ElementClassSummary | null,
	fallbacks: JsonObject,
	onChange: (attrs: JsonObject) => void
): PropertySource {
	const def = (name: string) => cls?.attributes[name];
	const defaultValue = (name: string) => def(name)?.default_value ?? fallbacks[name];
	return {
		get: (name) => attrs[name] ?? defaultValue(name) ?? undefined,
		isSet: (name) => name in attrs,
		fixed: (name) => def(name)?.scope === 'class' || def(name)?.instance_modifiable === false,
		required: (name) => def(name)?.default_value === null,
		helpTip: (name) => localized(def(name)?.help_tip),
		defaultValue: (name) => defaultValue(name) ?? undefined,
		set: (name, value) => onChange(value === undefined ? without(attrs, name) : { ...attrs, [name]: value }),
		names: Object.keys(cls?.attributes ?? {}),
		resettable: () => true
	};
}

// The page descriptor; page_layouter is set by choosing a page layouter class.
export function pageSource(page: JsonObject, onChange: (page: JsonObject) => void, onLayouter: (id: string) => void): PropertySource {
	return {
		...none,
		get: (name) => page[name] ?? undefined,
		isSet: (name) => name in page,
		set: (name, value) => {
			if (name === 'page_layouter') {
				if (typeof value === 'string') onLayouter(value);
				return;
			}
			onChange(value === undefined ? without(page, name) : { ...page, [name]: value });
		},
		names: Object.keys(page).filter((k) => k !== 'page_layouter' && k !== 'panels'),
		resettable: (name) => name !== 'page_layouter'
	};
}

// One pane of the page's tree; renaming it moves its panel and elements.
export function paneSource(page: JsonObject, paneId: string, onChange: (page: JsonObject) => void, onRename: (to: string) => void): PropertySource {
	const pane = (findPane(rootOf(page), paneId) ?? { pane_id: paneId }) as unknown as JsonObject;
	return {
		...none,
		get: (name) => pane[name] ?? undefined,
		isSet: (name) => name in pane,
		set: (name, value) => {
			if (name === 'pane_id') {
				if (typeof value === 'string' && value !== paneId) onRename(value);
				return;
			}
			onChange(setPaneAttr(page, paneId, name, value));
		},
		names: [],
		resettable: (name) => name !== 'pane_id'
	};
}

export function paneIsLeaf(page: JsonObject, paneId: string): boolean {
	const pane = findPane(rootOf(page), paneId);
	return !!pane && isLeaf(pane);
}

// A leaf pane's panel: its colours beside elem_layouter, the flow layouter's
// parameters in the panel's own config (else its layouter copy's).
export function panelSource(page: JsonObject, paneId: string, onChange: (page: JsonObject) => void): PropertySource {
	const panel = ((page.panels as JsonObject | undefined)?.[paneId] ?? {}) as JsonObject;
	const layouter = (panel.elem_layouter ?? { layouter: 'flow-layouter' }) as JsonObject;
	const config = (layouter.config ?? (layouter.class as JsonObject | undefined)?.config ?? {}) as JsonObject;
	const isFlow = (name: string) => name in flowDefaults;
	return {
		...none,
		get: (name) => {
			if (name === 'layouter') return layouter.layouter ?? undefined;
			if (isFlow(name)) return config[name] ?? flowDefaults[name as FlowField];
			return panel[name] ?? undefined;
		},
		isSet: (name) => (isFlow(name) ? name in config : name in panel),
		defaultValue: (name) => (isFlow(name) ? flowDefaults[name as FlowField] : undefined),
		set: (name, value: Json | undefined) => {
			if (isFlow(name)) onChange(setPanelConfig(page, paneId, value === undefined ? without(config, name) : { ...config, [name]: value }));
			else onChange(setPanelAttr(page, paneId, name, value));
		},
		names: [],
		resettable: () => true
	};
}
