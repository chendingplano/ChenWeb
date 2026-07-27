function syncAttributes(target: Element, source: Element) {
	for (const attribute of Array.from(target.attributes)) {
		if (!source.hasAttribute(attribute.name)) {
			target.removeAttribute(attribute.name);
		}
	}
	for (const attribute of Array.from(source.attributes)) {
		target.setAttribute(attribute.name, attribute.value);
	}
}

function parsePage(container: HTMLElement, svg: string): Element | null {
	const template = container.ownerDocument.createElement('template');
	template.innerHTML = svg.trim();
	return template.content.firstElementChild;
}

function updatePage(page: HTMLElement, svg: string) {
	const incoming = parsePage(page, svg);
	if (!incoming) return;

	const mounted = page.firstElementChild;
	if (!mounted || mounted.tagName !== incoming.tagName) {
		page.replaceChildren(incoming);
		return;
	}

	syncAttributes(mounted, incoming);
	mounted.replaceChildren(...Array.from(incoming.childNodes));
}

/**
 * Applies completed SVG pages synchronously while preserving existing page
 * wrappers and SVG roots. The browser therefore never paints an empty
 * preview between the old and new render.
 */
export function updatePreviewPages(container: HTMLElement, pages: string[]) {
	for (let i = 0; i < pages.length; i++) {
		let page = container.children.item(i) as HTMLElement | null;
		if (!page) {
			page = container.ownerDocument.createElement('div');
			page.className = 'cdm-shell-preview-page';
			container.append(page);
		}
		updatePage(page, pages[i]);
	}

	while (container.children.length > pages.length) {
		container.lastElementChild?.remove();
	}
}
