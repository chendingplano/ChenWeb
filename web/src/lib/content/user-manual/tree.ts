import type {
	Document,
	DocumentSource,
	DocumentTreeNode,
	MarkdownDocument
} from '$lib/documents/types';

const files = import.meta.glob('./**/*.md', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

function markdownFor(path: string): string {
	const content = files[path];
	if (content === undefined) {
		throw new Error(`User's Manual: missing content file ${path}`);
	}
	return content;
}

const tree: DocumentTreeNode[] = [
	{
		id: 'user-manual-getting-started',
		label: 'Getting Started',
		children: [
			{ id: 'user-manual-introduction', label: 'Introduction' },
			{ id: 'user-manual-installation', label: 'Installation' }
		]
	},
	{ id: 'user-manual-navigating-the-dashboard', label: 'Navigating the Dashboard' },
	{ id: 'user-manual-resources', label: 'Resources' }
];

const documents: Record<string, MarkdownDocument> = {
	'user-manual-introduction': {
		type: 'markdown',
		id: 'user-manual-introduction',
		markdown: markdownFor('./getting-started/introduction.md')
	},
	'user-manual-installation': {
		type: 'markdown',
		id: 'user-manual-installation',
		markdown: markdownFor('./getting-started/installation.md')
	},
	'user-manual-navigating-the-dashboard': {
		type: 'markdown',
		id: 'user-manual-navigating-the-dashboard',
		markdown: markdownFor('./navigating-the-dashboard.md')
	},
	'user-manual-resources': {
		type: 'markdown',
		id: 'user-manual-resources',
		markdown: markdownFor('./resources.md')
	}
};

export const userManualSource: DocumentSource = {
	listTree(): DocumentTreeNode[] {
		return tree;
	},
	getDocument(id: string): Document | undefined {
		return documents[id];
	}
};
