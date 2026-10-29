// Descriptor shapes for WPI schema_version "0.2" (apps, pages, action classes)
// and "0.4" (element classes), and the API responses.
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
	table?: string;
	result?: string;
	fields?: { column: string; attribute: string }[];
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
export type ElementInstance = Omit<SizeAttrs, 'width' | 'height'> & {
	width?: string | number;
	height?: string | number;
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
	pagination?: 'yes' | 'no';
	on_row_click?: ActionInstance;
	items?: MenuItem[];
	initial_item?: string;
};

// A class row: class attributes, and for each instance attribute either its
// default or null, meaning every instance must set it (action classes, format
// 0.2), or an object {help_tip, default_value, instance_modifiable} with the same
// meaning of default_value, where instance_modifiable false fixes the value for
// every instance (element classes, format 0.4). An element class names the code
// module that draws it in code_module_id.
export type ClassRow = {
	descriptor_id: string;
	schema_version: string;
	description?: Text;
	implement_notes?: Text;
	code_module_id?: string;
	[attr: string]: unknown;
};

// A page (devdoc §3.2.1). Its sizes place it inside its pane; binding_mode is
// the default of its elements that show data.
export type FlowLayouterConfig = {
	column_gap?: string;
	row_gap?: string;
	horizontal_alignment?: 'start' | 'center' | 'end' | 'space-between';
	vertical_alignment?: 'top' | 'center' | 'bottom' | 'stretch';
	rows_alignment?: 'top' | 'center' | 'bottom' | 'space-between';
};

export type PageDescriptor = SizeAttrs & {
	min_height?: string;
	max_height?: string;
	left_margin?: string;
	right_margin?: string;
	top_margin?: string;
	bottom_margin?: string;
	hori_align?: 'left' | 'center' | 'right';
	vert_align?: 'top' | 'center' | 'bottom';
	bkgd_color?: string;
	frgd_color?: string;
	access_controls?: string[];
	keywords?: string[];
	page_tags?: string[];
	page_policy?: string;
	page_style?: string;
	descriptor_id: string;
	schema_version: string;
	layouter: string;
	layouter_config?: FlowLayouterConfig;
	elements: ElementInstance[];
	page_name?: Text;
	description?: Text;
	input?: Input;
	save_action?: ActionInstance;
	refresh?: { on_show?: boolean };
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
