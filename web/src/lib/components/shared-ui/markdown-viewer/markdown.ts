// Safe Markdown → HTML for display. See
// KnowledgeStore/doc-repo/modules/module-markdown-viewer.md for usage.
import { Marked } from 'marked';

function escapeHtml(text: string): string {
	return text.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!);
}

export interface MarkdownOptions {
	// GitHub-flavoured Markdown (tables, task lists, strikethrough). Default true.
	gfm?: boolean;
	// A single newline becomes <br>. Default false.
	breaks?: boolean;
	// Allow image tags with http(s) sources. Default false (alt text is shown).
	allowImages?: boolean;
}

const SAFE_LINK = /^(https?:|mailto:|#)/i;
const SAFE_IMAGE = /^https?:/i;

const cache = new Map<string, Marked>();

function engine(opts: MarkdownOptions): Marked {
	const gfm = opts.gfm ?? true;
	const breaks = opts.breaks ?? false;
	const images = opts.allowImages ?? false;
	const key = `${gfm}|${breaks}|${images}`;
	let m = cache.get(key);
	if (!m) {
		m = new Marked({
			gfm,
			breaks,
			renderer: {
				// Source text may come from an LLM or a document: raw HTML is shown as text.
				html({ text }) {
					return escapeHtml(text);
				},
				link({ href, title, tokens }) {
					const body = this.parser.parseInline(tokens);
					if (!SAFE_LINK.test(href)) return body;
					const t = title ? ` title="${escapeHtml(title)}"` : '';
					const external = href.startsWith('#') ? '' : ' target="_blank" rel="noopener noreferrer"';
					return `<a href="${escapeHtml(href)}"${t}${external}>${body}</a>`;
				},
				image({ href, title, text }) {
					if (!images || !SAFE_IMAGE.test(href)) return escapeHtml(text);
					const t = title ? ` title="${escapeHtml(title)}"` : '';
					return `<img src="${escapeHtml(href)}" alt="${escapeHtml(text)}"${t} loading="lazy">`;
				}
			}
		});
		cache.set(key, m);
	}
	return m;
}

/** Renders Markdown to HTML that is safe for `{@html}`: no raw HTML, only http(s)/mailto/# links. */
export function renderMarkdown(source: string, opts: MarkdownOptions = {}): string {
	return engine(opts).parse(source ?? '', { async: false }) as string;
}
