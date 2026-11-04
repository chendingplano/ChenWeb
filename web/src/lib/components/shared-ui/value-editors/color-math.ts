export function rgbToHsv(r: number, g: number, b: number): [number, number, number] {
	r /= 255;
	g /= 255;
	b /= 255;
	const max = Math.max(r, g, b),
		min = Math.min(r, g, b),
		d = max - min;
	let h =
		d === 0 ? 0 : max === r ? ((g - b) / d) % 6 : max === g ? (b - r) / d + 2 : (r - g) / d + 4;
	h = (h * 60 + 360) % 360;
	return [h, max === 0 ? 0 : d / max, max];
}
export function hsvToRgb(h: number, s: number, v: number): [number, number, number] {
	const c = v * s,
		x = c * (1 - Math.abs(((h / 60) % 2) - 1)),
		m = v - c;
	const rgb =
		h < 60
			? [c, x, 0]
			: h < 120
				? [x, c, 0]
				: h < 180
					? [0, c, x]
					: h < 240
						? [0, x, c]
						: h < 300
							? [x, 0, c]
							: [c, 0, x];
	return rgb.map((n) => Math.round((n + m) * 255)) as [number, number, number];
}
export function rgbaHex(r: number, g: number, b: number, a: number): string {
	return (
		'#' +
		[r, g, b, ...(a < 1 ? [Math.round(a * 255)] : [])]
			.map((n) =>
				Math.max(0, Math.min(255, Math.round(n)))
					.toString(16)
					.padStart(2, '0')
			)
			.join('')
	);
}
