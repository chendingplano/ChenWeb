// The timestamp identifies a recovery record; the author still chooses the final page ID.
export function unnamedPageID(now: Date, used: Iterable<string>): string {
	const occupied = new Set(used);
	const date = new Date(now);
	const two = (n: number) => String(n).padStart(2, '0');
	for (;;) {
		const id = `unnamed-${date.getFullYear()}${two(date.getMonth() + 1)}${two(date.getDate())}-${two(date.getHours())}${two(date.getMinutes())}${two(date.getSeconds())}`;
		if (!occupied.has(id)) return id;
		date.setTime(date.getTime() + 1000);
	}
}
