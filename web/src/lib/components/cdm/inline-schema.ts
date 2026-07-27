// The ProseMirror schema for editing one CDM block's inline content
// (ADR 2026072603 DR1/DR7). Contains exactly CDM's eight inline types and no
// presentation mark (font, size, colour, alignment) -- the schema is the
// enforcement mechanism for D1's "no presentation properties" rule: a mark
// this file never defines cannot be produced by any editor action or paste,
// regardless of what the DOM or a pasted document contains.
//
// | CDM Inline.type   | ProseMirror representation                        |
// |--------------------|----------------------------------------------------|
// | text               | text node                                          |
// | strong/emphasis/link | marks wrapping arbitrary nested content (CDM's own
// |                    | `content: Inline[]` nesting), so multiple can combine on one run |
// | code               | a mark that EXCLUDES all others -- see below       |
// | math/citation/cross_reference | atom nodes ("chips"), their CDM fields |
// |                    | carried as opaque node attributes, not walked as children |
//
// `code` is CDM's odd one out: unlike strong/emphasis/link, which wrap
// `content: Inline[]` (arbitrary nested inline), CDM's `code` inline node is
// a LEAF carrying `text` directly (see server/api/cdm/rendering/typst.go's
// `case "code": return fmt.Sprintf("#raw(%q)", n.Text)` and every fixture:
// `{"type": "code", "text": "x := 1"}`, never `{"type": "code", "content": [...]}`).
// There is nothing to nest inside it, so its ProseMirror mark is given
// `excludes: '_'` (excludes every other mark): a code-marked run can never
// simultaneously carry strong/emphasis/link, which is exactly what CDM's
// leaf shape already implies. That exclusion is what makes the PM -> CDM
// direction of inline-mapping.ts unambiguous, not an incidental editor nicety.
import { Node, Mark, getSchema, type Extensions } from '@tiptap/core';
import type { Schema } from '@tiptap/pm/model';

const CdmDoc = Node.create({
	name: 'doc',
	topNode: true,
	// Flat: this schema's document IS one CDM block's inline content, never a
	// sequence of blocks (design D7). No paragraph wrapper node exists here.
	content: 'inline*'
});

const CdmText = Node.create({
	name: 'text',
	group: 'inline'
});

const CdmStrong = Mark.create({
	name: 'strong',
	parseHTML() {
		return [
			{ tag: 'strong' },
			{ tag: 'b' },
			{ style: 'font-weight', getAttrs: (v) => /^(bold|[5-9]00)$/.test(v as string) && null }
		];
	},
	renderHTML({ HTMLAttributes }) {
		return ['strong', HTMLAttributes, 0];
	},
	addKeyboardShortcuts() {
		return { 'Mod-b': () => this.editor.commands.toggleMark(this.name) };
	}
});

const CdmEmphasis = Mark.create({
	name: 'emphasis',
	parseHTML() {
		return [{ tag: 'em' }, { tag: 'i' }, { style: 'font-style=italic' }];
	},
	renderHTML({ HTMLAttributes }) {
		return ['em', HTMLAttributes, 0];
	},
	addKeyboardShortcuts() {
		return { 'Mod-i': () => this.editor.commands.toggleMark(this.name) };
	}
});

const CdmLink = Mark.create({
	name: 'link',
	inclusive: false,
	addAttributes() {
		return { url: { default: null } };
	},
	parseHTML() {
		return [
			{ tag: 'a[href]', getAttrs: (el) => ({ url: (el as HTMLElement).getAttribute('href') }) }
		];
	},
	renderHTML({ HTMLAttributes }) {
		return ['a', { ...HTMLAttributes, href: HTMLAttributes.url }, 0];
	}
});

const CdmCode = Mark.create({
	name: 'code',
	// See file-level comment: CDM's code inline node is a leaf, so it cannot
	// coexist with strong/emphasis/link in this schema.
	excludes: '_',
	parseHTML() {
		return [{ tag: 'code' }];
	},
	renderHTML({ HTMLAttributes }) {
		return ['code', HTMLAttributes, 0];
	},
	addKeyboardShortcuts() {
		return { 'Mod-e': () => this.editor.commands.toggleMark(this.name) };
	}
});

/** Shared config for the three atom ("chip") nodes: inline, atomic, selectable, and carrying no children -- their CDM data rides as opaque attrs instead. */
function atomNode(name: string, attrs: Record<string, unknown>) {
	return Node.create({
		name,
		group: 'inline',
		inline: true,
		atom: true,
		selectable: true,
		addAttributes() {
			return Object.fromEntries(Object.entries(attrs).map(([k, v]) => [k, { default: v }]));
		},
		parseHTML() {
			return [{ tag: `span[data-cdm-atom="${name}"]` }];
		},
		renderHTML({ HTMLAttributes }) {
			return ['span', { ...HTMLAttributes, 'data-cdm-atom': name }, 0];
		}
	});
}

const CdmMath = atomNode('math', { equation: null });
const CdmCitation = atomNode('citation', { citation_key: null, locator: null });
const CdmCrossReference = atomNode('cross_reference', { target: null, content: null });

export const INLINE_EXTENSIONS: Extensions = [
	CdmDoc,
	CdmText,
	CdmStrong,
	CdmEmphasis,
	CdmLink,
	CdmCode,
	CdmMath,
	CdmCitation,
	CdmCrossReference
];

let cachedSchema: Schema | null = null;

/** The ProseMirror Schema built from INLINE_EXTENSIONS, without instantiating a live Editor (no DOM required). Cached: extensions are static. */
export function buildInlineSchema(): Schema {
	if (!cachedSchema) {
		cachedSchema = getSchema(INLINE_EXTENSIONS);
	}
	return cachedSchema;
}
