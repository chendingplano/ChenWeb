// Client-side block ID allocation (ADR 2026072603 DR5, spec 2026072502 D9).
//
// Block IDs are minted here, not by the server: the client is what creates a
// block, and only it has the heading text or other context that makes a
// slug readable. The server does not trust what it receives — model.Validate
// rejects empty and duplicate IDs, and Store.Save maps a
// (document_id, block_id) unique violation to a typed conflict error naming
// the slug (see CdmBlockConflictError in cdm-client.ts). This module is the
// readable half of that split, not the enforcement.
//
// Once assigned, a block's id must never change (D9): editing content never
// re-slugs it, because anchors, cross_references, and artifact provenance
// all bind to the id, not to the block's position or text.

import type { Block } from './types.js';

const SLUG_STRIP_RE = /[^a-z0-9]+/g;
const MAX_SLUG_LENGTH = 60;

/**
 * Turns arbitrary text into a URL/id-safe slug. Mirrors
 * server/api/cdmhandler/handler.go's slugifyTitle (non-alphanumeric runs
 * collapse to a single hyphen), kept independent rather than shared because
 * one runs in Go against a title and this one runs in TypeScript against a
 * block's heading text — same algorithm, different callers, and there is no
 * shared module between the two languages to place it in.
 *
 * ASCII-oriented like its Go counterpart: non-Latin text (CJK, for one)
 * strips to empty and falls back to the type-based slug below.
 */
export function slugify(text: string): string {
	const s = text
		.toLowerCase()
		.trim()
		.replace(SLUG_STRIP_RE, '-')
		.replace(/^-+|-+$/g, '');
	return s.slice(0, MAX_SLUG_LENGTH).replace(/-+$/, '');
}

/** What a new block needs in order to be given a readable id. */
export interface BlockIdHint {
	/** The block's CDM type, used when no heading text is available or it strips to empty. */
	type: string;
	/** For a heading block, its plain-text content; ignored for other types. */
	headingText?: string;
}

/**
 * Allocates an id for a new block, unique against existingIds. Prefers a
 * slug derived from heading text; falls back to the block type. Appends a
 * numeric suffix only on collision, so the common case — one heading named
 * "Score Range" — stays exactly "score-range".
 */
export function allocateBlockId(hint: BlockIdHint, existingIds: ReadonlySet<string>): string {
	const fromHeading = hint.headingText ? slugify(hint.headingText) : '';
	const base = fromHeading || hint.type;

	if (!existingIds.has(base)) {
		return base;
	}
	for (let i = 2; i < 10000; i++) {
		const candidate = `${base}-${i}`;
		if (!existingIds.has(candidate)) {
			return candidate;
		}
	}
	throw new Error(`could not allocate a block id for base "${base}" after 10000 attempts`);
}

/**
 * Collects every block id in a document's block tree, including nested
 * children (callout, definition, ...) and list items. Used to build the
 * existingIds set allocateBlockId needs when inserting into a loaded
 * document.
 */
export function collectBlockIds(blocks: readonly Block[]): Set<string> {
	const ids = new Set<string>();
	const walk = (list: readonly Block[]) => {
		for (const b of list) {
			if (b.id) ids.add(b.id);
			if (b.children) walk(b.children);
			if (b.items) for (const item of b.items) walk(item);
		}
	};
	walk(blocks);
	return ids;
}

/**
 * Returns a stateful allocator seeded with existingIds, so a caller minting
 * several ids in one operation (e.g. a new `list` block plus its first
 * item) does not hand out the same slug twice. Each call both allocates and
 * reserves the id, in order.
 */
export function createIdAllocator(existingIds: ReadonlySet<string>): (hint: BlockIdHint) => string {
	const used = new Set(existingIds);
	return (hint) => {
		const id = allocateBlockId(hint, used);
		used.add(id);
		return id;
	};
}
