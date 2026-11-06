import type { FlowLayouterConfig, FlowSeparator } from '../types';

// The browser determines natural line breaks. Insert a flex break only when a
// natural line would exceed the configured count, then draw in the gaps.
export function flowGeometry(
	node: HTMLElement,
	initial: { config: FlowLayouterConfig; count: number; sizeToRows?: boolean; sizeTarget?: HTMLElement }
) {
	let options = initial;
	let frame = 0;
	let rowWidth = '';
	const originalWhiteSpace = new WeakMap<HTMLElement, string>();
	const clear = () => node.querySelectorAll(':scope > [data-flow-decoration]').forEach((n) => n.remove());
	const line = (
		kind: FlowSeparator | undefined,
		left: number,
		top: number,
		width: number,
		height: number
	) => {
		if (!kind || kind === 'none') return;
		const [weight, style] = kind.split('_');
		const size = weight === 'thin' ? 1 : weight === 'norm' ? 2 : 3;
		const el = document.createElement('div');
		el.dataset.flowDecoration = '';
		el.setAttribute('aria-hidden', 'true');
		Object.assign(el.style, {
			position: 'absolute',
			pointerEvents: 'none',
			left: `${left}px`,
			top: `${top}px`,
			width: `${width}px`,
			height: `${height}px`,
			boxSizing: 'border-box',
			borderTop:
				height === 0 ? `${size}px ${style === 'dash' ? 'dashed' : 'solid'} currentColor` : '',
			borderLeft:
				width === 0 ? `${size}px ${style === 'dash' ? 'dashed' : 'solid'} currentColor` : ''
		});
		node.appendChild(el);
	};
	const layout = () => {
		frame = 0;
		clear();
		const target = options.sizeTarget ?? node;
		if (options.sizeToRows) target.style.removeProperty('width');
		const items = Array.from(node.querySelectorAll<HTMLElement>(':scope > [data-flow-item]'));
		node.style.rowGap = '0px';
		const rowAlignment = options.config.rows_alignment ?? 'top';
		node.style.alignContent = {
			top: 'flex-start',
			center: 'safe center',
			bottom: 'safe flex-end',
			'space-between': 'flex-start'
		}[rowAlignment];
		items.forEach((item) => {
			item.style.marginBottom = '0px';
		});
		for (const item of items) {
			const text = item.classList.contains('flow-text')
				? item.querySelector<HTMLElement>('.body-row p')
				: null;
			if (!text) continue;
			text.style.removeProperty('-webkit-line-clamp');
			text.style.removeProperty('display');
			text.style.removeProperty('-webkit-box-orient');
			text.style.removeProperty('overflow');
			if (originalWhiteSpace.has(text)) {
				text.style.whiteSpace = originalWhiteSpace.get(text) ?? '';
				originalWhiteSpace.delete(text);
			}
			const maxWidth = parseFloat(getComputedStyle(item).maxWidth);
			if (item.clientWidth >= maxWidth - 1 && text.scrollWidth > text.clientWidth + 1) {
				originalWhiteSpace.set(text, text.style.whiteSpace);
				text.style.whiteSpace = 'normal';
			}
			const lineHeight = parseFloat(getComputedStyle(text).lineHeight);
			const maxHeight = parseFloat(getComputedStyle(item).maxHeight);
			const available =
				item.clientHeight - (text.getBoundingClientRect().top - item.getBoundingClientRect().top);
			if (lineHeight > 0 && item.clientHeight >= maxHeight - 1 && text.scrollHeight > available + 1) {
				text.style.display = '-webkit-box';
				text.style.setProperty('-webkit-box-orient', 'vertical');
				text.style.setProperty(
					'-webkit-line-clamp',
					String(Math.max(1, Math.floor(available / lineHeight)))
				);
				text.style.overflow = 'hidden';
				text.title = text.textContent ?? '';
				text.tabIndex = 0;
			} else {
				text.removeAttribute('title');
				text.removeAttribute('tabindex');
			}
		}
		const limit = options.config.max_num_elems ?? 50;
		let previousLeft = -Infinity,
			rowBottom = -Infinity,
			count = 0;
		for (const item of items) {
			if (item.offsetLeft <= previousLeft || item.offsetTop >= rowBottom) {
				count = 0;
				rowBottom = -Infinity;
			}
			if (limit > 0 && count >= limit) {
				const br = document.createElement('div');
				br.dataset.flowDecoration = '';
				br.setAttribute('aria-hidden', 'true');
				br.style.cssText = 'flex-basis:100%;width:0;height:0;min-height:0;margin:0;padding:0';
				node.insertBefore(br, item);
				count = 0;
				rowBottom = -Infinity;
			}
			count++;
			previousLeft = item.offsetLeft;
			rowBottom = Math.max(rowBottom, item.offsetTop + item.offsetHeight);
		}
		const rows: HTMLElement[][] = [];
		for (const item of items) {
			const previous = rows.at(-1);
			// Item top can differ with vertical alignment; use its flex line via
			// overlapping vertical ranges instead of equality of top coordinates.
			if (
				!previous ||
				item.offsetLeft <= previous.at(-1)!.offsetLeft ||
				item.offsetTop >= Math.max(...previous.map((e) => e.offsetTop + e.offsetHeight))
			)
				rows.push([item]);
			else previous.push(item);
		}
		rows
			.slice(0, -1)
			.flat()
			.forEach((item) => {
				item.style.marginBottom = options.config.row_gap ?? '0.75rem';
			});
		if (rowAlignment === 'space-between' && rows.length > 1) {
			const last = rows.at(-1)!;
			const used = Math.max(...last.map((e) => e.offsetTop + e.offsetHeight));
			const extra = Math.max(0, node.clientHeight - used) / (rows.length - 1);
			if (extra)
				rows
					.slice(0, -1)
					.flat()
					.forEach((item) => {
						item.style.marginBottom = `${parseFloat(getComputedStyle(item).marginBottom) + extra}px`;
					});
		}
		// A flex container's intrinsic width ignores forced row breaks. Size the
		// panel to the widest actual row before the pane positions it.
		// offsetLeft/offsetWidth are rounded; items are often fractional wide, so
		// a width taken from them can be a fraction short and wrap the last item
		// after the row gaps were measured. Use rects (undoing any canvas zoom)
		// and round up, plus 1px for the rounding error in the zoom scale.
		if (options.sizeToRows && rows.length) {
			const scale = node.offsetWidth ? node.getBoundingClientRect().width / node.offsetWidth : 1;
			const width = Math.ceil(Math.max(...rows.map((row) => {
				const rects = row.map((item) => item.getBoundingClientRect());
				return (Math.max(...rects.map((r) => r.right)) - Math.min(...rects.map((r) => r.left))) / scale;
			})) + 1);
			rowWidth = `${width}px`;
			target.style.width = rowWidth;
		}
		const bounds = rows.map((row) => ({
			left: Math.min(...row.map((e) => e.offsetLeft)),
			right: Math.max(...row.map((e) => e.offsetLeft + e.offsetWidth)),
			top: Math.min(...row.map((e) => e.offsetTop)),
			bottom: Math.max(...row.map((e) => e.offsetTop + e.offsetHeight))
		}));
		const maxWidth = Math.max(0, ...bounds.map((b) => b.right - b.left));
		rows.forEach((row, i) => {
			const b = bounds[i];
			for (let j = 1; j < row.length; j++) {
				const prev = row[j - 1],
					next = row[j];
				line(
					options.config.elem_separator,
					(prev.offsetLeft + prev.offsetWidth + next.offsetLeft) / 2,
					b.top,
					0,
					b.bottom - b.top
				);
			}
			if (i)
				line(
					options.config.row_separator,
					Math.min(...bounds.map((r) => r.left)),
					(bounds[i - 1].bottom + b.top) / 2,
					maxWidth,
					0
				);
		});
	};
	const schedule = () => {
		cancelAnimationFrame(frame);
		frame = requestAnimationFrame(layout);
	};
	const observer = new ResizeObserver(schedule);
	observer.observe(node);
	if ((options.sizeTarget ?? node).parentElement) observer.observe((options.sizeTarget ?? node).parentElement!);
	const observed = new WeakSet<Element>();
	const observeItems = () =>
		node.querySelectorAll('[data-flow-item]').forEach((item) => {
			if (!observed.has(item)) {
				observed.add(item);
				observer.observe(item);
			}
		});
	const textObserver = new MutationObserver(schedule);
	textObserver.observe(node, { subtree: true, characterData: true });
	observeItems();
	schedule();
	return {
		update(next: typeof initial) {
			if (options.sizeToRows && !next.sizeToRows && (options.sizeTarget ?? node).style.width === rowWidth) (options.sizeTarget ?? node).style.removeProperty('width');
			options = next;
			observeItems();
			schedule();
		},
		destroy() {
			observer.disconnect();
			textObserver.disconnect();
			cancelAnimationFrame(frame);
			clear();
		}
	};
}
