/** Choose a separate rounded Total scale for each chart, targeting eight gridlines. */
export function codeMonitorTotalScale(totals: number[]): { baseline: number; interval: number; maximum: number } {
	const minimum = totals.length ? Math.min(...totals) : 0;
	const maximum = totals.length ? Math.max(...totals) : 0;
	// Give constant series a small window centered on their value.
	const padding = minimum === maximum ? Math.max(3, Math.ceil(maximum * 0.005)) : 0;
	const lower = Math.max(0, minimum - padding);
	const upper = Math.max(maximum + padding, lower + 6);
	const magnitude = 10 ** Math.floor(Math.log10(Math.max(1, (upper - lower) / 6)));
	let best = { baseline: 0, interval: 1, maximum: 7 };
	let bestScore = Infinity;
	for (const factor of [1, 1.5, 2, 2.5, 3, 4, 5, 6, 7.5, 10]) {
		const interval = Math.max(1, Math.ceil(factor * magnitude));
		const baseline = Math.max(0, (Math.ceil(lower / interval) - 1) * interval);
		const ceiling = (Math.floor(upper / interval) + 1) * interval;
		const lines = (ceiling - baseline) / interval + 1;
		// Prefer eight lines; break ties with the tightest enclosing range.
		const score = Math.abs(lines - 8) + (ceiling - baseline) / Math.max(1, upper - lower) * 0.01;
		if (score < bestScore) {
			bestScore = score;
			best = { baseline, interval, maximum: ceiling - baseline };
		}
	}
	return best;
}
