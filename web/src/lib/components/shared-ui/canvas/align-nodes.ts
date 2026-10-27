// web/src/lib/components/shared-ui/canvas/align-nodes.ts
//
// Align and distribute the selected nodes of a Svelte Flow canvas. Pure functions over
// the node array: unselected nodes are returned unchanged, and a call that has too few
// selected nodes to act on returns the input array itself.

export type AlignMode = 'left' | 'center' | 'right' | 'top' | 'middle' | 'bottom';
export type DistributeAxis = 'horizontal' | 'vertical';

export type AlignableNode = {
	id: string;
	position: { x: number; y: number };
	selected?: boolean;
	width?: number;
	height?: number;
	measured?: { width?: number; height?: number };
};

type Box = { x: number; y: number; w: number; h: number };

function box(n: AlignableNode): Box {
	return {
		x: n.position.x,
		y: n.position.y,
		w: n.measured?.width ?? n.width ?? 0,
		h: n.measured?.height ?? n.height ?? 0
	};
}

export function selectedCount(nodes: AlignableNode[]): number {
	return nodes.filter((n) => n.selected).length;
}

// Align needs two selected nodes.
export function alignNodes<T extends AlignableNode>(nodes: T[], mode: AlignMode): T[] {
	const boxes = nodes.filter((n) => n.selected).map(box);
	if (boxes.length < 2) return nodes;

	const left = Math.min(...boxes.map((b) => b.x));
	const right = Math.max(...boxes.map((b) => b.x + b.w));
	const top = Math.min(...boxes.map((b) => b.y));
	const bottom = Math.max(...boxes.map((b) => b.y + b.h));

	return nodes.map((n) => {
		if (!n.selected) return n;
		const b = box(n);
		let { x, y } = n.position;
		switch (mode) {
			case 'left':   x = left; break;
			case 'center': x = (left + right) / 2 - b.w / 2; break;
			case 'right':  x = right - b.w; break;
			case 'top':    y = top; break;
			case 'middle': y = (top + bottom) / 2 - b.h / 2; break;
			case 'bottom': y = bottom - b.h; break;
		}
		return { ...n, position: { x, y } };
	});
}

// Distribute needs three selected nodes. The first and last node along the axis stay
// where they are; the ones between are moved so the gaps between neighbours are equal.
export function distributeNodes<T extends AlignableNode>(nodes: T[], axis: DistributeAxis): T[] {
	const horizontal = axis === 'horizontal';
	const picked = nodes
		.filter((n) => n.selected)
		.map((n) => ({ id: n.id, b: box(n) }))
		.sort((a, b) => (horizontal ? a.b.x - b.b.x : a.b.y - b.b.y));
	if (picked.length < 3) return nodes;

	const size = (b: Box) => (horizontal ? b.w : b.h);
	const start = horizontal ? picked[0].b.x : picked[0].b.y;
	const last = picked[picked.length - 1].b;
	const end = (horizontal ? last.x : last.y) + size(last);
	const used = picked.reduce((sum, p) => sum + size(p.b), 0);
	const gap = (end - start - used) / (picked.length - 1);

	const target = new Map<string, number>();
	let cursor = start;
	for (const p of picked) {
		target.set(p.id, cursor);
		cursor += size(p.b) + gap;
	}

	return nodes.map((n) => {
		const at = target.get(n.id);
		if (at === undefined) return n;
		return {
			...n,
			position: horizontal ? { x: at, y: n.position.y } : { x: n.position.x, y: at }
		};
	});
}
