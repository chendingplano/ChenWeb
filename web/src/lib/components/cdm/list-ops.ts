// Structural operations on a `list` block's items (task 6.2): each item is
// itself a `Block[]` (spec §1.2: "list uses items"), so adding/removing an
// item operates on the outer `items: Block[][]` array; editing what's
// *inside* one item reuses block-ops.ts's insertBlockAt/deleteBlockById on
// that item's own Block[] -- an item is not a different kind of thing from
// the top-level block array, just a nested one.
import type { Block } from './types.js';
import type { BlockIdHint } from './block-id.js';
import { createDefaultBlock } from './block-defaults.js';

/** Appends a new item containing one default paragraph. */
export function addListItem(block: Block, allocateId: (hint: BlockIdHint) => string): Block {
	const items = [...(block.items ?? []), [createDefaultBlock('paragraph', allocateId)]];
	return { ...block, items };
}

export function removeListItem(block: Block, index: number): Block {
	const items = (block.items ?? []).filter((_, i) => i !== index);
	return { ...block, items };
}
