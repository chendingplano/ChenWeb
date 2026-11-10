import { describe, expect, test } from 'bun:test';
import {
	docbuilderFileUrl,
	findLeaf,
	firstLeaf,
	stripFrontMatter,
	type DocTreeNode
} from './document-builder-types';

const tree: DocTreeNode[] = [
	{
		id: 'cat:A',
		label: 'A',
		children: [{ id: 'doc:a/x.md', label: 'X', kind: 'markdown', path: 'a/x.md' }]
	},
	{
		id: 'cat:B',
		label: 'B',
		children: [{ id: 'doc:y.pdf', label: 'Y', kind: 'pdf', path: 'y.pdf' }]
	}
];

describe('document builder tree helpers', () => {
	test('finds the first and a named leaf', () => {
		expect(firstLeaf(tree)?.path).toBe('a/x.md');
		expect(findLeaf(tree, 'y.pdf')?.kind).toBe('pdf');
		expect(findLeaf(tree, 'nope')).toBeUndefined();
		expect(firstLeaf([])).toBeUndefined();
	});
	test('encodes the file path', () => {
		expect(docbuilderFileUrl('s k', 'a b/c&d.md')).toBe(
			'/api/v1/docbuilder/s%20k/file?path=a%20b%2Fc%26d.md'
		);
	});

	test('strips front matter only at the start', () => {
		expect(stripFrontMatter('---\ntitle: A\nx: y\n---\n# Body\n')).toBe('# Body\n');
		expect(stripFrontMatter('# Body\n---\nnot: front\n---\n')).toBe(
			'# Body\n---\nnot: front\n---\n'
		);
		expect(stripFrontMatter('---\nno end')).toBe('---\nno end');
	});
});
