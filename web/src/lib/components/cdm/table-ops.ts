// Structural operations on a `table` block (task 6.1): column add/remove/
// retitle/align, row add/remove. Pure, like block-ops.ts -- each function
// takes a block and returns a new one; the caller (TableEditor.svelte)
// merges the result back onto the live reactive block object rather than
// replacing the array element BlockList owns, since these operations are
// scoped to one block's internal structure, not the document's top-level
// block list.
import type { Block, TableColumn, TableRow, Inline } from './types.js';

const ALPHABET = 'abcdefghijklmnopqrstuvwxyz';

/** Finds an unused single-letter column key, falling back to letter+number once the alphabet is exhausted. */
function nextColumnKey(existing: ReadonlySet<string>): string {
	for (const letter of ALPHABET) {
		if (!existing.has(letter)) return letter;
	}
	for (let i = 2; i < 1000; i++) {
		for (const letter of ALPHABET) {
			const candidate = `${letter}${i}`;
			if (!existing.has(candidate)) return candidate;
		}
	}
	throw new Error('table-ops: could not allocate a column key');
}

export function addColumn(block: Block, title = 'New Column'): Block {
	const columns = block.columns ?? [];
	const key = nextColumnKey(new Set(columns.map((c) => c.key)));
	const newColumns: TableColumn[] = [...columns, { key, title }];
	const rows: TableRow[] = (block.rows ?? []).map((row) => ({
		cells: { ...row.cells, [key]: [{ type: 'text', text: '' }] }
	}));
	return { ...block, columns: newColumns, rows };
}

export function removeColumn(block: Block, key: string): Block {
	const columns = (block.columns ?? []).filter((c) => c.key !== key);
	const rows: TableRow[] = (block.rows ?? []).map((row) => {
		const cells = { ...row.cells };
		delete cells[key];
		return { cells };
	});
	return { ...block, columns, rows };
}

export function renameColumnTitle(block: Block, key: string, title: string): Block {
	const columns = (block.columns ?? []).map((c) => (c.key === key ? { ...c, title } : c));
	return { ...block, columns };
}

/** An empty align clears the column back to the spec default ("left"), by omitting the key entirely rather than setting it to an empty string -- CDM's align is absent-or-a-value, never "". */
export function setColumnAlign(block: Block, key: string, align: string): Block {
	const columns = (block.columns ?? []).map((c) => {
		if (c.key !== key) return c;
		if (!align) {
			const { align: _drop, ...rest } = c;
			return rest;
		}
		return { ...c, align };
	});
	return { ...block, columns };
}

export function addRow(block: Block): Block {
	const columns = block.columns ?? [];
	const cells: Record<string, Inline[]> = {};
	for (const col of columns) cells[col.key] = [{ type: 'text', text: '' }];
	const rows: TableRow[] = [...(block.rows ?? []), { cells }];
	return { ...block, rows };
}

export function removeRow(block: Block, index: number): Block {
	const rows = (block.rows ?? []).filter((_, i) => i !== index);
	return { ...block, rows };
}
