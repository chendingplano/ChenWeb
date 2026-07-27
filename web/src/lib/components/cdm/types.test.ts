import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

import type {
	Document,
	Metadata,
	Block,
	Inline,
	TableColumn,
	TableRow,
	Equation,
	MathSource,
	MathExpr,
	RefTarget
} from './types.js';

// This is the regression guard design D8 calls for: these types must have
// "the same field names and shapes as the Go struct's JSON encoding," since
// the editor's document state is meant to be this shape directly, with no
// conversion layer between it and what the API sends.
//
// TypeScript cannot validate JSON.parse's output against an interface at
// runtime, so a naive "parse into a typed variable" test would pass even if
// this file's field names were wrong. Instead, each clone* function below
// touches only the fields its corresponding interface declares; rebuilding a
// fixture through them and comparing the result to the original JSON is what
// makes a misnamed or missing field visible — either as a TypeScript compile
// error (reading a property this file didn't declare) or as a mismatched
// assertion (a field this file declared under the wrong name silently reads
// as undefined and gets dropped from the rebuilt object).

function cloneMathExpr(e: MathExpr): MathExpr {
	return {
		op: e.op,
		args: e.args?.map(cloneMathExpr),
		type: e.type,
		name: e.name,
		value: e.value
	};
}

function cloneMathSource(s: MathSource): MathSource {
	return { format: s.format, source: s.source };
}

function cloneEquation(eq: Equation): Equation {
	return {
		display: eq.display,
		original: eq.original ? cloneMathSource(eq.original) : undefined,
		normalized: eq.normalized ? cloneMathExpr(eq.normalized) : undefined,
		parse_status: eq.parse_status
	};
}

function cloneRefTarget(t: RefTarget): RefTarget {
	return { document_key: t.document_key, block_id: t.block_id };
}

function cloneInline(n: Inline): Inline {
	return {
		type: n.type,
		text: n.text,
		content: n.content?.map(cloneInline),
		url: n.url,
		math: n.math ? cloneEquation(n.math) : undefined,
		target: n.target ? cloneRefTarget(n.target) : undefined,
		citation_key: n.citation_key,
		locator: n.locator
	};
}

function cloneTableColumn(c: TableColumn): TableColumn {
	return { key: c.key, title: c.title, align: c.align };
}

function cloneTableRow(r: TableRow): TableRow {
	const cells: Record<string, Inline[]> = {};
	for (const [key, inlines] of Object.entries(r.cells)) {
		cells[key] = inlines.map(cloneInline);
	}
	return { cells };
}

function cloneBlock(b: Block): Block {
	return {
		id: b.id,
		type: b.type,
		role: b.role,
		level: b.level,
		title: b.title,
		term: b.term,
		text: b.text,
		lang: b.lang,
		ordered: b.ordered,
		items: b.items?.map((item) => item.map(cloneBlock)),
		content: b.content?.map(cloneInline),
		children: b.children?.map(cloneBlock),
		columns: b.columns?.map(cloneTableColumn),
		rows: b.rows?.map(cloneTableRow),
		math: b.math ? cloneEquation(b.math) : undefined,
		src: b.src,
		alt: b.alt,
		caption: b.caption?.map(cloneInline)
	};
}

function cloneMetadata(m: Metadata): Metadata {
	return {
		doc_type: m.doc_type,
		rendering_type: m.rendering_type,
		authors: m.authors ? [...m.authors] : undefined,
		version: m.version,
		create_time: m.create_time,
		modify_time: m.modify_time
	};
}

function cloneDocument(d: Document): Document {
	return {
		document_key: d.document_key,
		title: d.title,
		language: d.language,
		schema_version: d.schema_version,
		content_version: d.content_version,
		edit_version: d.edit_version,
		metadata: cloneMetadata(d.metadata),
		blocks: d.blocks.map(cloneBlock)
	};
}

const here = path.dirname(fileURLToPath(import.meta.url));

function assertRoundTrips(fixtureFile: string) {
	const raw = readFileSync(path.join(here, 'testdata', fixtureFile), 'utf8');
	const parsed: Document = JSON.parse(raw);
	const rebuilt = cloneDocument(parsed);
	// Route the rebuilt object back through JSON so undefined-valued fields
	// (present as own properties on the clone, but never present in real
	// JSON) drop out the same way Go's `omitempty` drops them — matching
	// what parsed already looks like.
	const normalized = JSON.parse(JSON.stringify(rebuilt));
	assert.deepStrictEqual(normalized, JSON.parse(raw));
}

test('jaro-winkler fixture round-trips through the TypeScript types', () => {
	assertRoundTrips('jaro-winkler.json');
});

test('all-block-types fixture round-trips through the TypeScript types (every Phase 1 block and inline type)', () => {
	assertRoundTrips('all-block-types.json');
});
