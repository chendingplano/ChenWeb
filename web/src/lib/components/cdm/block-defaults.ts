// Default shapes for newly inserted blocks (block list task 4.1/4.3).
//
// Each Phase 1 block type needs *some* minimal valid shape the moment it is
// inserted, or there is nothing for the editor to show. These are the
// smallest shapes that satisfy model.Validate's invariants (spec §1.2):
// content XOR children XOR items, table cell keys drawn from declared
// columns, an equation carrying at least original or normalized.

import type { Block, BlockType } from './types.js';
import type { BlockIdHint } from './block-id.js';

/** Mints a new, empty block of type, allocating its id (and any nested ids) via allocateId. */
export function createDefaultBlock(
	type: BlockType,
	allocateId: (hint: BlockIdHint) => string
): Block {
	const id = allocateId({ type });

	switch (type) {
		case 'paragraph':
		case 'quote':
			return { id, type, content: [{ type: 'text', text: '' }] };

		case 'heading':
			return { id, type, level: 2, content: [{ type: 'text', text: '' }] };

		case 'list':
			return {
				id,
				type,
				ordered: false,
				items: [[createDefaultBlock('paragraph', allocateId)]]
			};

		case 'table':
			return {
				id,
				type,
				columns: [
					{ key: 'a', title: 'Column 1' },
					{ key: 'b', title: 'Column 2' }
				],
				rows: [
					{
						cells: {
							a: [{ type: 'text', text: '' }],
							b: [{ type: 'text', text: '' }]
						}
					}
				]
			};

		case 'equation':
			return {
				id,
				type,
				math: { display: true, parse_status: 'skipped', original: { format: 'typst', source: '' } }
			};

		case 'code':
			return { id, type, lang: '', text: '' };

		case 'image':
			return { id, type, src: '', alt: '' };

		case 'callout':
			return {
				id,
				type,
				role: 'note',
				children: [createDefaultBlock('paragraph', allocateId)]
			};

		default:
			// Exhaustiveness guard: if BLOCK_TYPES in types.ts ever grows, this
			// throws loudly rather than silently inserting an empty shell.
			throw new Error(`no default shape defined for block type "${type}"`);
	}
}
