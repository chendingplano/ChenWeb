/** Rounded baseline below every visible total, with precision set by the maximum. */
export function codeMonitorTotalBaseline(totals: number[]): number {
	if (totals.length === 0) return 0;
	const maximum = Math.max(...totals);
	const minimum = Math.min(...totals);
	if (maximum <= 0 || minimum <= 0) return 0;
	const step = Math.max(1, 10 ** (Math.floor(Math.log10(maximum)) - 1));
	return Math.max(0, (Math.ceil(minimum / step) - 1) * step);
}
