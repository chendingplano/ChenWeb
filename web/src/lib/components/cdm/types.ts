// CDM v1.0 canonical document AST, mirrored field-for-field from the Go
// definitions in server/api/cdm/model/types.go and server/api/cdm/model/validate.go.
//
// Field names match the JSON encoding exactly (snake_case, not camelCase),
// so that these types are what the API sends and receives with no
// translation layer in between (design D8): the editor's document state is
// data shaped like this, and saving is JSON.stringify, not a conversion.
//
// See types.test.ts for the round-trip check that catches drift between this
// file and the Go source.

/** The CDM schema version this file implements (server/api/cdm/model: SchemaVersion). */
export const SCHEMA_VERSION = '1.0';

/** Phase 1 block type vocabulary (validate.go: blockTypes). */
export const BLOCK_TYPES = [
	'paragraph',
	'heading',
	'list',
	'table',
	'equation',
	'code',
	'quote',
	'image',
	'callout'
] as const;
export type BlockType = (typeof BLOCK_TYPES)[number];

/** Phase 1 inline type vocabulary (validate.go: inlineTypes). */
export const INLINE_TYPES = [
	'text',
	'strong',
	'emphasis',
	'code',
	'link',
	'math',
	'citation',
	'cross_reference'
] as const;
export type InlineType = (typeof INLINE_TYPES)[number];

/** Callout severity vocabulary (validate.go: calloutRoles). */
export const CALLOUT_ROLES = ['note', 'tip', 'important', 'warning', 'caution'] as const;
export type CalloutRole = (typeof CALLOUT_ROLES)[number];

/** Equation source formats (validate.go: equationFormats). */
export const EQUATION_FORMATS = ['latex', 'typst', 'asciimath'] as const;
export type EquationFormat = (typeof EQUATION_FORMATS)[number];

/** Equation parse outcomes (validate.go: parseStatuses). */
export const PARSE_STATUSES = ['success', 'failed', 'skipped'] as const;
export type ParseStatus = (typeof PARSE_STATUSES)[number];

/** Table column alignment; empty/absent means "left" (types.go: TableColumn.Align). */
export const COLUMN_ALIGNS = ['left', 'center', 'right'] as const;
export type ColumnAlign = (typeof COLUMN_ALIGNS)[number];

/** Document is the root canonical document (spec §1, §5.2). */
export interface Document {
	document_key: string;
	title: string;
	language: string;
	schema_version: string;
	content_version: number;
	edit_version?: number;
	metadata: Metadata;
	blocks: Block[];
}

/** Document-level metadata (spec §4.1). */
export interface Metadata {
	doc_type?: string;
	rendering_type?: string;
	authors?: string[];
	version?: string;
	/** RFC 3339 / ISO 8601 UTC, absent when zero (Go: omitzero). */
	create_time?: string;
	modify_time?: string;
}

/**
 * Block is the single shape for every block type. Which fields are populated
 * is determined by `type`; see CDM spec §1.2 for the content-model
 * invariants this mirrors (content XOR children XOR items, etc.) — the
 * invariants are enforced server-side by model.Validate, not by this type.
 */
export interface Block {
	id: string;
	type: BlockType | string;

	/** Callout severity only (spec §3). */
	role?: CalloutRole | string;

	/** Heading level, 1-6 (heading only). */
	level?: number;

	/** Callout title (callout only). */
	title?: string;

	/** The term being defined (definition only; not in the Phase 1 vocabulary yet). */
	term?: string;

	/** Verbatim source text (code only). */
	text?: string;
	/** Highlight language (code only). */
	lang?: string;

	/** Ordered vs. unordered list (list only). */
	ordered?: boolean;
	/** List items; each item is itself a list of blocks (list only). */
	items?: Block[][];

	/** Inline payload (paragraph, heading, quote). */
	content?: Inline[];

	/** Nested blocks (callout, definition, and other semantic containers). */
	children?: Block[];

	/** Table columns and rows (table only). */
	columns?: TableColumn[];
	rows?: TableRow[];

	/** Equation (equation only). */
	math?: Equation;

	/** Image source and alt text (image only). */
	src?: string;
	alt?: string;
	/**
	 * Rendered as a Typst #figure caption (image and table only;
	 * ADR 2026072602 DR5d wraps a table in #figure so it is numbered and
	 * listable in the List of Tables regardless of whether it has one).
	 */
	caption?: Inline[];
}

/** One inline content node. */
export interface Inline {
	type: InlineType | string;
	text?: string;
	content?: Inline[];

	/** Link target (link only). */
	url?: string;

	/** Inline math (math only). */
	math?: Equation;

	/** Block target (cross_reference only). */
	target?: RefTarget;

	/** Bibliography entry key and locator (citation only). */
	citation_key?: string;
	locator?: string;
}

/** One declared column of a table block. */
export interface TableColumn {
	key: string;
	title: string;
	/** "left" | "center" | "right"; absent means "left". */
	align?: ColumnAlign | string;
}

/**
 * One row of a table block. Cells are keyed by TableColumn.key; a key
 * absent from cells renders as an empty cell (spec §1.2 rule 5).
 */
export interface TableRow {
	cells: Record<string, Inline[]>;
}

/**
 * The CDM v1.0 equation shape (spec §6.4): store the original source, store
 * a normalized semantic math AST when parsing succeeds, and render from the
 * AST when available, falling back to the original representation
 * otherwise.
 */
export interface Equation {
	display: boolean;
	original?: MathSource;
	normalized?: MathExpr;
	parse_status: ParseStatus | string;
}

/** An equation's source as originally authored. */
export interface MathSource {
	format: EquationFormat | string;
	source: string;
}

/**
 * One node of a normalized math AST: either an operator node (op + args) or
 * a leaf (type + name/value). Numeric literals are carried as exact decimal
 * strings so they round-trip without binary floating-point loss (spec §6.4).
 */
export interface MathExpr {
	op?: string;
	args?: MathExpr[];

	/** "symbol" | "number" (leaf only). */
	type?: string;
	/** Symbol name (leaf only). */
	name?: string;
	/** Number value, as an exact decimal string (leaf only). */
	value?: string;
}

/** Addresses a block, optionally in another document. An absent document_key means "this document" (spec §7). */
export interface RefTarget {
	document_key?: string;
	block_id: string;
}
