import { describe, expect, test } from 'bun:test';
import { renderMarkdown } from './markdown';

describe('renderMarkdown', () => {
	test('renders headings, emphasis and GFM tables', () => {
		const html = renderMarkdown('# T\n\n**b** *i*\n\n| a | b |\n|---|---|\n| 1 | 2 |');
		expect(html).toContain('<h1>T</h1>');
		expect(html).toContain('<strong>b</strong>');
		expect(html).toContain('<table>');
	});
	test('raw HTML is shown as text', () => {
		const html = renderMarkdown('<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>');
		expect(html).not.toContain('<script');
		expect(html).not.toContain('<img');
		expect(html).toContain('&lt;script&gt;');
	});
	test('only http(s), mailto and # links survive', () => {
		expect(renderMarkdown('[x](javascript:alert(1))')).not.toContain('href');
		expect(renderMarkdown('[x](https://a.b)')).toContain('rel="noopener noreferrer"');
		expect(renderMarkdown('[x](mailto:a@b.c)')).toContain('href="mailto:a@b.c"');
		expect(renderMarkdown('[x](#s)')).not.toContain('target=');
	});
	test('images are off by default and http(s)-only when allowed', () => {
		expect(renderMarkdown('![alt](https://a.b/i.png)')).not.toContain('<img');
		expect(renderMarkdown('![alt](https://a.b/i.png)', { allowImages: true })).toContain('<img src="https://a.b/i.png"');
		expect(renderMarkdown('![alt](data:text/html;base64,AA)', { allowImages: true })).not.toContain('<img');
	});
	test('options and blank input', () => {
		expect(renderMarkdown('a\nb', { breaks: true })).toContain('<br>');
		expect(renderMarkdown('a\nb')).not.toContain('<br>');
		expect(renderMarkdown('')).toBe('');
	});
});
