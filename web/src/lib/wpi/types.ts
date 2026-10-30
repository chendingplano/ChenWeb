// Descriptor shapes for WPI schema_version "0.5" (apps, pages, layouter
// classes), "0.4" (element classes) and "0.2" (action classes), and the API
// responses (OpenSpec wpi-pages-own-panes).
// The Go validator (server/api/wpi) is the authority on what is valid; these
// types describe what the interpreter receives in a validated bundle. A stored
// page holds a copy of each class it uses in "class"; a page being edited may
// not have them yet, so "class" is optional here.

export type Json = string | number | boolean | null | Json[] | { [key: string]: Json };
export type JsonObject = { [key: string]: Json };

// One pane of a page layouter's tree (devdoc 2026102601 §3.2.2). A pane with
// split is divided into children; one without is a leaf and holds a panel.
export type PaneSpec = {
	pane_id: string;
	split?: 'rows' | 'columns';
	children?: PaneSpec[];
	width?: string;
	height?: string;
	min_width?: string;
	max_width?: string;
	min_height?: string;
	max_height?: string;
	resizable?: boolean;
	left_margin?: string;
	right_margin?: string;
	top_margin?: string;
	bottom_margin?: string;
	hori_align?: 'left' | 'center' | 'right';
	vert_align?: 'top' | 'center' | 'bottom';
	hori_scrollbar?: 'auto' | 'hide';
	vert_scrollbar?: 'auto' | 'hide';
};

// An app opens its initial page in its page area.
export type AppDescriptor = {
	descriptor_id: string;
	schema_version: string;
	initial_page: string;
};

// Text shown the same in every language, or one text per language code.
export type Text = string | Record<string, string>;

// An action instance, written where it is used and named by its action class
// (devdoc 2026102601 §5). The attributes of load, save, query and load-file are
// read only by the server.
export type ActionInstance = {
	action: string;
	class?: ClassRow;
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
	class?: ClassRow;
	text?: Text;
	label?: Text;
	data_binding?: string;
	binding_mode?: 'read-only' | 'read-write';
	input_type?: 'text' | 'number' | 'email' | 'password' | 'tel' | 'url' | 'search';
	placeholder?: Text;
	help_text?: Text;
	help_tip?: Text;
	required?: boolean;
	max_length?: number;
	validation?: { min_length?: number; pattern?: string; min?: number; max?: number; step?: number };
	autocomplete?: string;
	label_placement?: 'left' | 'right' | 'top' | 'bottom';
	label_horizontal_gap?: string;
	label_vertical_gap?: string;
	label_horizontal_align?: 'left' | 'center' | 'right' | 'start' | 'end';
	label_vertical_align?: 'top' | 'center' | 'bottom';
	label_max_width?: string;
	label_max_lines?: number;
	label_font_family?: string;
	label_font_size?: string;
	label_color?: string;
	label_font_weight?: string | number;
	label_font_style?: 'normal' | 'italic' | 'oblique';
	icon?: string;
	icon_placement?: 'start' | 'end';
	icon_size?: string;
	icon_gap?: string;
	icon_color?: string;
	font_family?: string;
	font_size?: string;
	font_weight?: string | number;
	font_style?: 'normal' | 'italic' | 'oblique';
	line_height?: 'normal' | number;
	color?: string;
	background_color?: string;
	text_align?: 'start' | 'center' | 'end' | 'justify';
	white_space?: 'normal' | 'pre-wrap';
	padding?: string;
	options?: { value: string | number; label: string }[];
	on_click?: ActionInstance;
	source?: ActionInstance;
	presentation?: string;
	columns?: ListColumn[];
	pagination?: 'yes' | 'no';
	on_row_click?: ActionInstance;
	items?: MenuItem[];
	initial_item?: string;
	// chat-composer and chat-panel (code_modules/interpreter-chat-*.md).
	rows?: number;
	slash_commands?: boolean;
	toolbar?: string[];
	agents?: string[];
	default_agent?: string;
	default_model?: string;
	tabs?: string[];
	initial_tab?: string;
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

export type FlowLayouterConfig = {
	column_gap?: string;
	row_gap?: string;
	horizontal_alignment?: 'start' | 'center' | 'end' | 'space-between';
	vertical_alignment?: 'top' | 'center' | 'bottom' | 'stretch';
	rows_alignment?: 'top' | 'center' | 'bottom' | 'space-between';
};

// A page's copy of a page layouter class: the whole class in class, and the
// page's own tree, which replaces the class's when set.
export type PageLayouterCopy = { layouter: string; class?: ClassRow & { root?: PaneSpec }; root?: PaneSpec };

// A panel's copy of an element layouter class: the whole class in class, and
// the panel's own parameters, which replace the class's when set.
export type ElementLayouterCopy = { layouter: string; class?: ClassRow & { config?: FlowLayouterConfig }; config?: FlowLayouterConfig };

// The elements shown in one leaf pane, and the element layouter that arranges
// them (devdoc §3.3).
export type Panel = { elem_layouter: ElementLayouterCopy; elem_list?: ElementInstance[] };

// A page (devdoc §3.2.3). Its sizes place it inside its page area: the app's,
// or the pane it was opened in; binding_mode is the default of its elements
// that show data.
export type PageDescriptor = SizeAttrs & {
	min_height?: string;
	max_height?: string;
	hori_align?: 'left' | 'center' | 'right';
	vert_align?: 'top' | 'center' | 'bottom';
	hori_scrollbar?: 'auto' | 'hide';
	vert_scrollbar?: 'auto' | 'hide';
	bkgd_color?: string;
	frgd_color?: string;
	access_controls?: string[];
	keywords?: string[];
	page_tags?: string[];
	page_policy?: string;
	page_style?: string;
	descriptor_id: string;
	schema_version: string;
	page_layouter: PageLayouterCopy;
	panels?: Record<string, Panel>;
	page_name?: Text;
	description?: Text;
	input?: Input;
	save_action?: ActionInstance;
	refresh?: { on_show?: boolean };
	binding_mode?: 'read-only' | 'read-write';
};

export type Issue = { kind: string; descriptor_id: string; path: string; message: string };

// An app and its pages, whose instances carry their class copies.
export type Bundle = {
	app: AppDescriptor;
	pages: Record<string, PageDescriptor>;
	warnings: Issue[];
};

// Names an action instance by its place in a stored page.
export type ActionRef = { kind: 'page'; id: string; pointer: string };

// A page instance's input: {} for an empty doc, static data, or the action instance
// that produces it and its argument values.
export type InputSpec = { ref?: ActionRef; args?: JsonObject; doc?: JsonObject };
