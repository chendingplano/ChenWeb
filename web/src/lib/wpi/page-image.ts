import { toBlob } from 'html-to-image';

// Page images (OpenSpec wpi-page-images): the editor captures the page as the
// author sees it and uploads it before the page is saved.

// The editor draws these on top of the page; they are not part of it.
const editorOnly = ['overlay', 'pane-tag', 'pane-divider', 'outer-edge', 'resize-handle', 'element-grip', 'element-resize-outline', 'reorder-line'];

// keepNode says whether a node of the page preview belongs in the image.
export function keepNode(node: Node): boolean {
	if (!(node instanceof Element)) return true;
	return !editorOnly.some((name) => node.classList.contains(name));
}

// capturePageImage renders the editor's page preview as a PNG.
export async function capturePageImage(surface: HTMLElement, backgroundColor: string): Promise<Blob> {
	const blob = await toBlob(surface, { pixelRatio: 1, cacheBust: true, backgroundColor, filter: keepNode });
	if (!blob) throw new Error('page image capture produced no image');
	return blob;
}

// uploadPageImage stores the image on the server and returns the name the page
// keeps in `page_image`.
export async function uploadPageImage(pageId: string, image: Blob, fetchFn: typeof fetch = fetch): Promise<string> {
	const res = await fetchFn(`/api/v1/wpi/page-images/${encodeURIComponent(pageId)}`, {
		method: 'PUT', credentials: 'same-origin', headers: { 'Content-Type': 'image/png' }, body: image
	});
	const body = await res.json().catch(() => ({}));
	if (!res.ok) throw new Error(String(body.error ?? res.statusText));
	return String(body.page_image);
}
