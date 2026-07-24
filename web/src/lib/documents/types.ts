export type DocumentType = 'markdown' | 'template-json' | 'typst' | 'html';

export interface MarkdownDocument {
	type: 'markdown';
	id: string;
	markdown: string;
}

export interface TemplateJsonDocument {
	type: 'template-json';
	id: string;
	templateId: string;
	data: Record<string, unknown>;
}

export interface TypstDocument {
	type: 'typst';
	id: string;
	source: string;
}

export interface HtmlDocument {
	type: 'html';
	id: string;
	html: string;
}

export type Document = MarkdownDocument | TemplateJsonDocument | TypstDocument | HtmlDocument;

export interface DocumentTreeNode {
	id: string;
	label: string;
	children?: DocumentTreeNode[];
}

export interface DocumentSource {
	listTree(): DocumentTreeNode[];
	getDocument(id: string): Document | undefined;
}
