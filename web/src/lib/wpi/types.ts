// Descriptor shapes for WPI schema_version "0.2" and the API responses.
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

// Text shown the same in every language, or one text per language code.
export type Text = string | Record<string, string>;

// `{}` as an input: open the page with an empty input doc.
export type EmptyInput = Record<string, never>;

// An action instance, written where it is used and named by its action class
// (devdoc 2026102601 §5). load and save attributes are read only by the server.
export type ActionInstance = {
	action: string;
	args?: JsonObject;
	page?: string;
	pane?: string;
	input?: ActionInstance | EmptyInput;
};

export type MenuItem = {
	item_id: string;
	label: string;
	on_click?: ActionInstance;
	children?: MenuItem[];
};

export type ListColumn = { field: string; label: string; width?: string };

export type SizeAttrs = { width?: string; height?: string; min_width?: string; max_width?: string };

// An element instance as written in a page, named by its element class
// (devdoc §3.5). Merged with its class row it is complete.
export type ElementInstance = SizeAttrs & {
	element: string;
	text?: string;
	label?: Text;
	data_binding?: string;
	binding_mode?: 'read-only' | 'read-write';
	input_type?: 'text' | 'number' | 'email';
	options?: { value: string | number; label: string }[];
	on_click?: ActionInstance;
	source?: ActionInstance;
	presentation?: string;
	columns?: ListColumn[];
	on_row_click?: ActionInstance;
	items?: MenuItem[];
	initial_item?: string;
};

// A class row: class attributes, and for instance attributes a default or
// null (every instance must set it).
export type ClassRow = { descriptor_id: string; schema_version: string; description?: Text; [attr: string]: unknown };

export type PageDescriptor = {
	descriptor_id: string;
	schema_version: string;
	layouter: string;
	elements: ElementInstance[];
	input?: ActionInstance | EmptyInput;
	save_action?: ActionInstance;
	refresh?: { on_show?: boolean };
};

export type Issue = { kind: string; descriptor_id: string; path: string; message: string };

// An app, its pages and the element and action classes they use.
export type Bundle = {
	app: AppDescriptor;
	pages: Record<string, PageDescriptor>;
	elements: Record<string, ClassRow>;
	actions: Record<string, ClassRow>;
	warnings: Issue[];
};

// Names an action instance by its place in a stored page.
export type ActionRef = { kind: 'page'; id: string; pointer: string };

// A panel's input: {} for an empty doc, or the load instance that produces it
// and its argument values.
export type InputSpec = { ref?: ActionRef; args?: JsonObject };
