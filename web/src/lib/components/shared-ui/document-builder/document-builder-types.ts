// Shape of GET /api/v1/docbuilder/:source/tree (server/api/docbuilder).
export type DocTreeNode = {
	id: string;
	label: string;
	kind?: 'markdown' | 'pdf';
	path?: string; // leaf only
	children?: DocTreeNode[];
};

export type DocTreeResponse = { title: string; tree: DocTreeNode[]; uncategorized: number };

export const docbuilderTreeUrl = (source: string) =>
	`/api/v1/docbuilder/${encodeURIComponent(source)}/tree`;
export const docbuilderFileUrl = (source: string, path: string) =>
	`/api/v1/docbuilder/${encodeURIComponent(source)}/file?path=${encodeURIComponent(path)}`;

// First leaf in tree order, used to open something when nothing was selected before.
export function firstLeaf(nodes: DocTreeNode[]): DocTreeNode | undefined {
	for (const n of nodes) {
		if (n.path) return n;
		const f = n.children && firstLeaf(n.children);
		if (f) return f;
	}
	return undefined;
}

export function findLeaf(nodes: DocTreeNode[], path: string): DocTreeNode | undefined {
	for (const n of nodes) {
		if (n.path === path) return n;
		const f = n.children && findLeaf(n.children, path);
		if (f) return f;
	}
	return undefined;
}

// Removes a leading YAML front-matter block (--- ... ---) so it is not shown as body text.
export function stripFrontMatter(src: string): string {
	const m = /^﻿?---\r?\n[\s\S]*?\r?\n---[ \t]*(?:\r?\n|$)/.exec(src);
	return m ? src.slice(m[0].length) : src;
}
