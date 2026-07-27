// Structural operations on a document's top-level block array (task 4.3).
//
// Every function here is pure: it takes a block array and returns a new one,
// so the block list component can assign the result straight back to its
// $state array and Svelte's reactivity picks it up. None of them touch a
// block's id once assigned (D9) — insertBlockAt and moveBlock never construct
// or rename a Block, and changeContentBlockType explicitly carries `id`
// through unchanged.

import type { Block } from './types.js';

export function insertBlockAt(blocks: readonly Block[], index: number, block: Block): Block[] {
	const copy = blocks.slice();
	copy.splice(index, 0, block);
	return copy;
}

export function deleteBlockById(blocks: readonly Block[], id: string): Block[] {
	return blocks.filter((b) => b.id !== id);
}

/** Swaps a block with its neighbor; a no-op (returns an equivalent copy) at either end of the array. */
export function moveBlock(blocks: readonly Block[], id: string, direction: 'up' | 'down'): Block[] {
	const idx = blocks.findIndex((b) => b.id === id);
	if (idx === -1) return blocks.slice();

	const target = direction === 'up' ? idx - 1 : idx + 1;
	if (target < 0 || target >= blocks.length) return blocks.slice();

	const copy = blocks.slice();
	[copy[idx], copy[target]] = [copy[target], copy[idx]];
	return copy;
}

/**
 * Block types whose id and inline content survive a type change unchanged.
 * Restricted to these three deliberately: `paragraph`/`heading`/`quote` are
 * the CDM block types that carry `content` (spec §1.2 rule 4), so converting
 * among them is a genuine in-place edit. Every other type has no meaningful
 * content-preserving equivalent — a table's columns/rows do not become
 * paragraph text — so changing into or out of one is done by deleting the
 * block and inserting a fresh default of the target type (createDefaultBlock
 * in block-defaults.ts), not by this function.
 */
export type ContentBearingType = 'paragraph' | 'heading' | 'quote';
const CONTENT_BEARING_TYPES = new Set<string>(['paragraph', 'heading', 'quote']);

export function isContentBearingType(type: string): type is ContentBearingType {
	return CONTENT_BEARING_TYPES.has(type);
}

/**
 * Converts a block in place among paragraph/heading/quote, preserving its id
 * and content. Converting to `heading` keeps an existing valid level or
 * defaults to 2; converting away from `heading` drops level, since paragraph
 * and quote carry none.
 */
export function changeContentBlockType(block: Block, newType: ContentBearingType): Block {
	if (!isContentBearingType(block.type)) {
		throw new Error(
			`cannot change type of block "${block.id}" (type "${block.type}"): only paragraph/heading/quote support in-place type change`
		);
	}
	if (newType === 'heading') {
		const level = block.level && block.level >= 1 && block.level <= 6 ? block.level : 2;
		return { id: block.id, type: newType, level, content: block.content };
	}
	return { id: block.id, type: newType, content: block.content };
}
