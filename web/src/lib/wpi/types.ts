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

// An action instance, written where it is used and named by its action class
// (devdoc 2026102601 §5). The attributes of load, save, query and load-file are
// read only by the server.
export type ActionInstance = {
	action: string;
	args?: JsonObject;
	page?: string;
	pane?: string;
	input?: Input;
};

// A page's or an open-page's input: an action instance that produces the doc,
// or static data, the doc itself ({} is the empty doc; devdoc §3.2.2).
export type Input = ActionInstance | JsonObject;

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

// A page (devdoc §3.2.1). Its sizes place it inside its pane; binding_mode is
// the default of its elements that show data.
export type PageDescriptor = SizeAttrs & {
	descriptor_id: string;
	schema_version: string;
	layouter: string;
	elements: ElementInstance[];
	page_name?: Text;
	description?: Text;
	input?: Input;
	save_action?: ActionInstance;
	refresh?: { on_show?: boolean };
	page_alignment?: 'left' | 'center' | 'right';
	binding_mode?: 'read-only' | 'read-write';
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

// A panel's input: {} for an empty doc, static data, or the action instance
// that produces it and its argument values.
export type InputSpec = { ref?: ActionRef; args?: JsonObject; doc?: JsonObject };
