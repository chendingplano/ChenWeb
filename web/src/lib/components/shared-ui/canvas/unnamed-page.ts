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

// Tab labels are unnamed-1, unnamed-2, …; the lowest number no open tab uses is reused.
export function unnamedTabLabel(used: Iterable<string>): string {
	const occupied = new Set(used);
	for (let n = 1; ; n++) if (!occupied.has(`unnamed-${n}`)) return `unnamed-${n}`;
}
