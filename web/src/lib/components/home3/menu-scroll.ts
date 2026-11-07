type Position = { element: HTMLElement; top: number; left: number };
export type ScrollSnapshot = { top: number; left: number; positions: Position[] };

export function captureScroll(main: HTMLElement, page: HTMLElement): ScrollSnapshot {
	const elements = [page, ...page.querySelectorAll<HTMLElement>('*')];
	return {
		top: main.scrollTop,
		left: main.scrollLeft,
		positions: elements.filter((el) => el.scrollTop || el.scrollLeft).map((element) => ({
			element, top: element.scrollTop, left: element.scrollLeft
		}))
	};
}

export function restoreScroll(main: HTMLElement, snapshot: ScrollSnapshot): void {
	main.scrollTop = snapshot.top;
	main.scrollLeft = snapshot.left;
	for (const { element, top, left } of snapshot.positions) {
		element.scrollTop = top;
		element.scrollLeft = left;
	}
}
