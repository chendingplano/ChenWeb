// Property editor definitions (OpenSpec wpi-property-editors, design D1): what
// the WPI editor's Properties panel shows for one kind of entity. A definition
// is data; one renderer (shared-ui/property-editor) draws every entity from its
// definition and a PropertySource that reads and writes the entity.
import type { ValueEditorOptions } from '$lib/components/shared-ui/value-editors/types';
import type { FlowLayouterConfig, Json } from '../types';
import type { ValidatorClass } from '../validators';

export type ValueEditorId =
	| 'text'
	| 'multiline-text'
	| 'localized-text'
	| 'number'
	| 'switch'
	| 'value-picker'
	| 'ordered-multi-picker'
	| 'string-list'
	| 'color'
	| 'length'
	| 'icon-picker'
	| 'emoji-picker'
	| 'table-picker'
	| 'field-picker'
	| 'data-source'
	| 'column-list'
	| 'sort-order-editor'
	| 'validation'
	| 'validator-list'
	| 'read-only'
	| 'json';

// What a property's definition may look at: the entity's other values and the
// lists the editor has loaded from the server.
export type PropertyContext = {
	get: (name: string) => Json | undefined;
	triggerActions: string[];
	pageLayouters: string[];
	// The validator classes an element's Validators group offers.
	validatorClasses?: ValidatorClass[];
	// Measured on the canvas, in page pixels from the page's top-left corner.
	position?: { x: number; y: number };
	// An element's pane and the flow layouter configuration of its panel.
	pane?: string;
	flowConfig?: FlowLayouterConfig;
	// A pane's: whether it is a leaf, and why a new pane ID cannot be used.
	leaf?: boolean;
	paneIdProblem?: (id: string) => string | null;
};

export type PropertyDef = {
	name: string;
	label?: () => string;
	// Page, pane and panel tips; an element's tip is its class's help_tip.
	helpTip?: () => string;
	// Omitted: the common-sense editor (common-sense.ts).
	editor?: ValueEditorId;
	// The data the value editor needs; a function when it depends on other values.
	options?: ValueEditorOptions | ((ctx: PropertyContext) => ValueEditorOptions);
	// Between the stored value and what the value editor edits.
	read?: (stored: Json | undefined) => Json | undefined;
	write?: (edited: Json | undefined) => Json | undefined;
	// Not an attribute: a value worked out from the canvas, always read-only.
	computed?: (ctx: PropertyContext) => Json | undefined;
	readOnly?: boolean;
	visible?: (ctx: PropertyContext) => boolean;
	// A message refuses the value (it is not applied); a warning is shown while it is.
	validate?: (value: Json, ctx: PropertyContext) => string | null;
	warn?: (value: Json, ctx: PropertyContext) => string | null;
	// A fixed note under the value editor.
	note?: () => string;
};

export type GroupDef = {
	id: string;
	label: () => string;
	help?: () => string;
	properties: PropertyDef[];
	visible?: (ctx: PropertyContext) => boolean;
};

// otherHelp: the note under the Other group, which holds attributes the definition does not list.
export type EditorDef = { groups: GroupDef[]; otherHelp?: () => string };

// Reads and writes one entity for the renderer. set(name, undefined) unsets.
export type PropertySource = {
	get(name: string): Json | undefined;
	isSet(name: string): boolean;
	fixed(name: string): boolean;
	required(name: string): boolean;
	helpTip(name: string): string;
	defaultValue(name: string): Json | undefined;
	set(name: string, value: Json | undefined): void;
	// Attributes the entity has besides those its definition lists (shown in Other).
	names: string[];
	// Whether Reset is offered for a property that has its own value.
	resettable(name: string): boolean;
};
