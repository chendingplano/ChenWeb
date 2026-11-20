/** Move a native dialog by its title bar and keep its resize corner in view. */
export function movableDialog(header: HTMLElement, anchor?: DOMRect) {
	const dialog = header.closest('dialog');
	if (!dialog) return;
	const panel = dialog;
	const margin = 8;
	let drag: { id: number; x: number; y: number } | undefined;

	function position(x: number, y: number) {
		const { width, height } = panel.getBoundingClientRect();
		const left = Math.max(margin, Math.min(x, window.innerWidth - width - margin));
		const top = Math.max(margin, Math.min(y, window.innerHeight - height - margin));
		Object.assign(panel.style, {
			inset: `${top}px auto auto ${left}px`,
			margin: '0',
			maxWidth: `calc(100vw - ${left + margin}px)`,
			maxHeight: `calc(100dvh - ${top + margin}px)`
		});
	}

	function start(event: PointerEvent) {
		if (event.button !== 0 || (event.target as Element).closest('button')) return;
		const rect = panel.getBoundingClientRect();
		position(rect.left, rect.top);
		drag = { id: event.pointerId, x: event.clientX - rect.left, y: event.clientY - rect.top };
		header.setPointerCapture(event.pointerId);
	}

	function move(event: PointerEvent) {
		if (drag?.id === event.pointerId) position(event.clientX - drag.x, event.clientY - drag.y);
	}

	function end(event: PointerEvent) {
		if (drag?.id === event.pointerId) drag = undefined;
	}

	function fit() {
		if (!panel.style.top) return;
		panel.style.maxWidth = 'calc(100vw - 16px)';
		panel.style.maxHeight = 'calc(100dvh - 16px)';
		const rect = panel.getBoundingClientRect();
		position(rect.left, rect.top);
	}

	// The native dialog must be open before its dimensions are available.
	const frame = requestAnimationFrame(() => {
		if (!anchor || !panel.open) return;
		const { width, height } = panel.getBoundingClientRect();
		const left = anchor.right + margin + width <= window.innerWidth
			? anchor.right + margin
			: anchor.left - width - margin;
		if (left >= margin) {
			position(left, anchor.top);
		} else {
			const below = anchor.bottom + margin;
			position(anchor.left, below + height <= window.innerHeight ? below : anchor.top - height - margin);
		}
	});
	header.addEventListener('pointerdown', start);
	header.addEventListener('pointermove', move);
	header.addEventListener('lostpointercapture', end);
	window.addEventListener('resize', fit);

	return {
		destroy() {
			cancelAnimationFrame(frame);
			header.removeEventListener('pointerdown', start);
			header.removeEventListener('pointermove', move);
			header.removeEventListener('lostpointercapture', end);
			window.removeEventListener('resize', fit);
		}
	};
}
