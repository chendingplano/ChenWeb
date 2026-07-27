// Bidirectional conversion between CDM's `Inline[]` (nested wrapper nodes:
// strong/emphasis/link each carry `content: Inline[]`) and a ProseMirror
// document built from inline-schema.ts's flat mark-based representation.
//
// The two directions are not symmetric in difficulty. CDM -> ProseMirror is a
// straightforward recursive descent: walk the nested tree, accumulate marks
// as wrapper nodes are entered. ProseMirror -> CDM is the harder direction,
// because ProseMirror's marks are an unordered set attached per-leaf, while
// CDM's wrappers are an ordered, nested tree shared across however many
// leaves the same marks apply to. Two adjacent bold runs must become one
// `strong` node containing two children, not two sibling `strong` nodes --
// see groupByMarks below.
import type { Schema, Node as PMNode, Mark as PMMark } from '@tiptap/pm/model';
import type { Inline } from './types.js';

/**
 * Drops undefined-valued keys from a fresh object. Needed because
 * `assert.deepStrictEqual` (and JSON semantics generally) treat a key present
 * with value `undefined` as different from the key being absent, and CDM's
 * JSON encoding always omits absent optional fields entirely (Go's
 * `omitempty`) rather than encoding them as `null`/`undefined`.
 */
function dropUndefined<T extends object>(obj: T): T {
	for (const key of Object.keys(obj) as (keyof T)[]) {
		if (obj[key] === undefined) delete obj[key];
	}
	return obj;
}

const WRAPPER_TYPES = ['strong', 'emphasis', 'link'] as const;
type WrapperType = (typeof WRAPPER_TYPES)[number];

function isWrapperType(type: string): type is WrapperType {
	return (WRAPPER_TYPES as readonly string[]).includes(type);
}

/** CDM Inline[] -> a ProseMirror `doc` node, per inline-schema.ts's flat single-block schema. */
export function inlineArrayToProseMirrorDoc(schema: Schema, inline: Inline[]): PMNode {
	return schema.nodes.doc.create(null, inlineArrayToPMNodes(schema, inline, []));
}

function inlineArrayToPMNodes(
	schema: Schema,
	inline: Inline[],
	activeMarks: readonly PMMark[]
): PMNode[] {
	const out: PMNode[] = [];
	for (const node of inline) {
		out.push(...inlineNodeToPMNodes(schema, node, activeMarks));
	}
	return out;
}

function inlineNodeToPMNodes(
	schema: Schema,
	node: Inline,
	activeMarks: readonly PMMark[]
): PMNode[] {
	switch (node.type) {
		case 'text':
			return node.text ? [schema.text(node.text, activeMarks as PMMark[])] : [];

		case 'line_break':
			return [schema.nodes.line_break.create()];

		case 'code':
			// code excludes every other mark at the schema level (see
			// inline-schema.ts), so it is applied alone regardless of
			// activeMarks -- a code leaf can never inherit an ancestor's
			// strong/emphasis/link in this schema.
			return node.text ? [schema.text(node.text, [schema.marks.code.create()])] : [];

		case 'strong':
		case 'emphasis':
		case 'link': {
			const attrs = node.type === 'link' ? { url: node.url ?? null } : undefined;
			const mark = schema.marks[node.type].create(attrs);
			return inlineArrayToPMNodes(
				schema,
				node.content ?? [],
				mark.addToSet(activeMarks as PMMark[])
			);
		}

		case 'math':
			return [
				schema.nodes.math.create(
					{ equation: node.math ?? null },
					undefined,
					activeMarks as PMMark[]
				)
			];

		case 'citation':
			return [
				schema.nodes.citation.create(
					{ citation_key: node.citation_key ?? null, locator: node.locator ?? null },
					undefined,
					activeMarks as PMMark[]
				)
			];

		case 'cross_reference':
			return [
				schema.nodes.cross_reference.create(
					{ target: node.target ?? null, content: node.content ?? null },
					undefined,
					activeMarks as PMMark[]
				)
			];

		default:
			// An inline type this schema does not model. Preserve the text if
			// there is any rather than silently dropping author content; this
			// keeps an unrecognized future type visible instead of invisible.
			return node.text ? [schema.text(node.text, activeMarks as PMMark[])] : [];
	}
}

/** A leaf (text or atom node) tagged with the ordered wrapper marks that apply to it, outermost first. */
interface TaggedLeaf {
	marks: WrapperType[];
	linkUrl: string | null;
	node: Inline;
}

// Fixed outer-to-inner order for reconstructing nested wrappers. Arbitrary
// but must be fixed for the output to be deterministic regardless of the
// order ProseMirror happens to store marks in.
const WRAPPER_ORDER: WrapperType[] = ['link', 'strong', 'emphasis'];

export function proseMirrorDocToInlineArray(doc: PMNode): Inline[] {
	const leaves: TaggedLeaf[] = [];
	doc.content.forEach((child) => {
		leaves.push(leafToTagged(child));
	});
	return groupByMarks(leaves, 0);
}

function leafToTagged(node: PMNode): TaggedLeaf {
	const hasCode = node.marks.some((m) => m.type.name === 'code');
	if (hasCode) {
		// code excludes every other mark by schema construction, so no
		// wrapper marks are possible here.
		return { marks: [], linkUrl: null, node: { type: 'code', text: node.text ?? '' } };
	}

	let linkUrl: string | null = null;
	const marks = WRAPPER_ORDER.filter((type) => {
		const mark = node.marks.find((m) => m.type.name === type);
		if (mark && type === 'link') linkUrl = (mark.attrs.url as string | null) ?? null;
		return Boolean(mark);
	});

	if (node.isText) {
		return { marks, linkUrl, node: { type: 'text', text: node.text ?? '' } };
	}
	if (node.type.name === 'line_break') {
		return { marks, linkUrl, node: { type: 'line_break' } };
	}
	if (node.type.name === 'math') {
		return {
			marks,
			linkUrl,
			node: dropUndefined({ type: 'math', math: node.attrs.equation ?? undefined })
		};
	}
	if (node.type.name === 'citation') {
		return {
			marks,
			linkUrl,
			node: dropUndefined({
				type: 'citation',
				citation_key: node.attrs.citation_key ?? undefined,
				locator: node.attrs.locator ?? undefined
			})
		};
	}
	if (node.type.name === 'cross_reference') {
		return {
			marks,
			linkUrl,
			node: dropUndefined({
				type: 'cross_reference',
				target: node.attrs.target ?? undefined,
				content: node.attrs.content ?? undefined
			})
		};
	}
	throw new Error(`inline-mapping: no CDM mapping for ProseMirror node type "${node.type.name}"`);
}

/**
 * Groups a flat leaf list into nested CDM wrapper nodes, merging adjacent
 * leaves that share the same mark at the current depth into one wrapper's
 * `content` rather than emitting one wrapper per leaf. depth indexes into
 * each leaf's `marks` array (WRAPPER_ORDER position, not the mark itself).
 */
function groupByMarks(leaves: TaggedLeaf[], depth: number): Inline[] {
	const result: Inline[] = [];
	let i = 0;
	while (i < leaves.length) {
		const mark = leaves[i].marks[depth];
		if (mark === undefined) {
			result.push(leaves[i].node);
			i++;
			continue;
		}
		let j = i + 1;
		while (j < leaves.length && leaves[j].marks[depth] === mark) j++;
		const group = leaves.slice(i, j);
		const content = groupByMarks(group, depth + 1);
		result.push(
			mark === 'link'
				? dropUndefined({ type: 'link', url: group[0].linkUrl ?? undefined, content })
				: { type: mark, content }
		);
		i = j;
	}
	return result;
}
