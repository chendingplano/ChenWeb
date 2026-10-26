// Descriptor shapes for WPI schema_version "0.1" and the API responses.
// The Go validator (server/api/wpi) is the authority on what is valid; these
// types describe what the interpreter receives in a validated bundle.

export type Json = string | number | boolean | null | Json[] | { [key: string]: Json };
export type JsonObject = { [key: string]: Json };

export type PaneSpec = {
	width?: string;
	height?: string;
	min_width?: string;
	max_width?: string;
	min_height?: string;
	max_height?: string;
	resizable?: boolean;
	scroll?: 'auto' | 'none';
	page?: string;
};

export type AppDescriptor = {
	descriptor_id: string;
	schema_version: string;
	layout_template: string;
	panes: Record<string, PaneSpec>;
};

// `{}` for an empty input doc, or a load action that produces it.
export type InputSpec = { action?: string; args?: JsonObject };

export type Trigger = {
	action?: string;
	action_type?: string;
	args?: JsonObject;
	page?: string;
	pane?: string;
	input?: InputSpec;
};

export type MenuItem = {
	item_id: string;
	label: string;
	on_click?: Trigger;
	children?: MenuItem[];
};

export type ListColumn = { field: string; label: string; width?: string };

export type SizeAttrs = { width?: string; height?: string; min_width?: string; max_width?: string };

export type ElementDescriptor = SizeAttrs & {
	descriptor_id: string;
	element_type: string;
	text?: string;
	label?: string;
	data_binding?: string;
	binding_mode?: 'read-only' | 'read-write';
	input_type?: 'text' | 'number' | 'email';
	options?: { value: string | number; label: string }[];
	on_click?: Trigger;
	source?: { action: string; args?: JsonObject };
	presentation?: string;
	columns?: ListColumn[];
	on_row_click?: Trigger;
	items?: MenuItem[];
	initial_item?: string;
};

// A page entry: an inline element, or a stored element by reference with overrides.
export type ElementEntry = ElementDescriptor | (Partial<ElementDescriptor> & { element: string });

export type PageDescriptor = {
	descriptor_id: string;
	schema_version: string;
	layouter: string;
	elements: ElementEntry[];
	input?: InputSpec;
	save_action?: string;
	refresh?: { on_show?: boolean };
};

export type ActionDescriptor = Trigger & { descriptor_id: string; action_type: string };

export type Issue = { kind: string; descriptor_id: string; path: string; message: string };

export type Bundle = {
	app: AppDescriptor;
	pages: Record<string, PageDescriptor>;
	elements: Record<string, ElementDescriptor>;
	actions: Record<string, ActionDescriptor>;
	warnings: Issue[];
};

// Names a stored action, or an inline action inside a stored page or element.
export type ActionRef = { kind: 'action' | 'page' | 'element'; id: string; pointer?: string };
