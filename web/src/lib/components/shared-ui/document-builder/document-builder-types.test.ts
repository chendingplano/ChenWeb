import { describe, expect, test } from 'bun:test';
import {
	docGroupFileUrl,
	findLeaf,
	firstLeaf,
	stripFrontMatter,
	type DocTreeNode
} from './document-builder-types';

const tree: DocTreeNode[] = [
	{
		id: 'cat:A',
		label: 'A',
		children: [{ id: 'doc:a/x.md', label: 'X', kind: 'markdown', doc_id: 11 }]
	},
	{
		id: 'cat:B',
		label: 'B',
		children: [{ id: 'doc:y.pdf', label: 'Y', kind: 'pdf', doc_id: 22 }]
	}
];

describe('document builder tree helpers', () => {
	test('finds the first and a named leaf', () => {
		expect(firstLeaf(tree)?.doc_id).toBe(11);
		expect(findLeaf(tree, 22)?.kind).toBe('pdf');
		expect(findLeaf(tree, 99)).toBeUndefined();
		expect(firstLeaf([])).toBeUndefined();
	});
	test('builds group URLs', () => {
		expect(docGroupFileUrl('s k', 7)).toBe('/api/v1/doc-groups/s%20k/documents/7/file');
	});

	test('strips front matter only at the start', () => {
		expect(stripFrontMatter('---\ntitle: A\nx: y\n---\n# Body\n')).toBe('# Body\n');
		expect(stripFrontMatter('# Body\n---\nnot: front\n---\n')).toBe(
			'# Body\n---\nnot: front\n---\n'
		);
		expect(stripFrontMatter('---\nno end')).toBe('---\nno end');
	});
});
