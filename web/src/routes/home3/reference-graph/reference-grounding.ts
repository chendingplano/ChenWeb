import type { RawLine } from '$lib/services/kbService';
import type { TableReference } from '$lib/components/home3/pdf-table-geometry';
import type { PdfPageViewport } from '$lib/components/home3/shared-pdf-viewer.svelte';
import { lineNumbersFromSpans } from '$lib/components/home3/metric-ontology-explorer/metric-source-spans';

export function resolveReferenceLines(spans: unknown, tables: TableReference[], lines: RawLine[], documentTitle = ''): RawLine[] {
	const numbers = new Set([...lineNumbersFromSpans(spans), ...tables.map(table => table.line)]);
	const ordered = [...lines].sort((a, b) => a.page_number - b.page_number || a.line_number - b.line_number);
	if (numbers.size > 0) return ordered.filter(line => numbers.has(line.line_number));
	// A whole-document citation points at the title, rather than an invented passage.
	const normalize = (text: string) => text.replace(/[\s\p{P}]/gu, '').toLocaleLowerCase();
	const title = normalize(documentTitle);
	const titleLine = title.length >= 4
		? ordered.find(line => normalize(line.content) === title) ?? ordered.find(line => normalize(line.content).includes(title))
		: undefined;
	return titleLine ? [titleLine] : [];
}

export function paintReferenceHighlights(lines: RawLine[], tables: TableReference[], page: number, viewport: PdfPageViewport, overlay: HTMLDivElement) {
	for (const line of lines) {
		if (line.page_number !== page || tables.some(table => table.line === line.line_number)) continue;
		if (line.coords?.length < 4 || !line.coords?.every(Number.isFinite)) continue;
		const [x1, y1, x2, y2] = line.coords;
		const width = Math.abs(x2 - x1) * viewport.width / 1000;
		const height = Math.abs(y2 - y1) * viewport.height / 1000;
		if (width < 1 || height < 1) continue;
		const mark = overlay.ownerDocument.createElement('div');
		mark.className = 'pdf-highlight';
		Object.assign(mark.style, {
			position: 'absolute', left: `${Math.min(x1, x2) * viewport.width / 1000}px`,
			top: `${Math.min(y1, y2) * viewport.height / 1000}px`, width: `${width}px`, height: `${height}px`,
			background: 'rgba(200,85,61,.18)', border: '1px solid rgba(200,85,61,.9)', pointerEvents: 'none'
		});
		overlay.appendChild(mark);
	}
}
