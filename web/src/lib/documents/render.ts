import { marked } from 'marked';
import type { Document } from './types';

export async function renderDocument(doc: Document): Promise<string> {
	switch (doc.type) {
		case 'markdown':
			return marked.parse(doc.markdown);
		case 'template-json':
			throw new Error(`Document type 'template-json' not yet supported`);
		case 'typst':
			throw new Error(`Document type 'typst' not yet supported`);
		case 'html':
			throw new Error(`Document type 'html' not yet supported`);
	}
}
