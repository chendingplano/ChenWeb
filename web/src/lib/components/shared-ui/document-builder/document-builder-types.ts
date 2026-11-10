// Shape of GET /api/v1/doc-groups/:group/tree (server/api/docbuilder).
export type DocTreeNode = {
	id: string;
	label: string;
	kind?: 'markdown' | 'pdf';
	doc_id?: number; // leaf only
	children?: DocTreeNode[];
};

export type DocTreeResponse = { title: string; tree: DocTreeNode[]; uncategorized: number };

export const docGroupTreeUrl = (group: string) =>
	`/api/v1/doc-groups/${encodeURIComponent(group)}/tree`;
export const docGroupFileUrl = (group: string, docId: number) =>
	`/api/v1/doc-groups/${encodeURIComponent(group)}/documents/${docId}/file`;
export const docGroupPdfUrl = (group: string, docId: number) =>
	`/api/v1/doc-groups/${encodeURIComponent(group)}/documents/${docId}/pdf`;

// First leaf in tree order, used to open something when nothing was selected before.
export function firstLeaf(nodes: DocTreeNode[]): DocTreeNode | undefined {
	for (const n of nodes) {
		if (n.doc_id) return n;
		const f = n.children && firstLeaf(n.children);
		if (f) return f;
	}
	return undefined;
}

export function findLeaf(nodes: DocTreeNode[], docId: number): DocTreeNode | undefined {
	for (const n of nodes) {
		if (n.doc_id === docId) return n;
		const f = n.children && findLeaf(n.children, docId);
		if (f) return f;
	}
	return undefined;
}

// Removes a leading YAML front-matter block (--- ... ---) so it is not shown as body text.
export function stripFrontMatter(src: string): string {
	const m = /^﻿?---\r?\n[\s\S]*?\r?\n---[ \t]*(?:\r?\n|$)/.exec(src);
	return m ? src.slice(m[0].length) : src;
}
