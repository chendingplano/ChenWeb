import { describe, expect, test } from 'bun:test';
import { parseHTML } from 'linkedom';
import { captureScroll, restoreScroll } from './menu-scroll';

describe('cached page scroll positions', () => {
	test('restores the page and nested scroll containers independently', () => {
		const { document } = parseHTML('<main><div><div></div></div></main>');
		const main = document.querySelector('main') as unknown as HTMLElement;
		const page = main.firstElementChild as HTMLElement;
		const nested = page.firstElementChild as HTMLElement;
		main.scrollTop = 90; nested.scrollTop = 42; nested.scrollLeft = 11;
		const snapshot = captureScroll(main, page);
		main.scrollTop = 0; nested.scrollTop = 0; nested.scrollLeft = 0;
		restoreScroll(main, snapshot);
		expect([main.scrollTop, nested.scrollTop, nested.scrollLeft]).toEqual([90, 42, 11]);
	});
});
