/** Put folders before pages within each section, retaining their configured order. */
export function foldersFirst<T extends { children?: readonly unknown[]; group?: string }>(items: T[]): T[] {
	const groups = new Map(items.map((item) => [item.group, 0]));
	let index = 0;
	for (const group of groups.keys()) groups.set(group, index++);
	return [...items].sort((a, b) =>
		groups.get(a.group)! - groups.get(b.group)! ||
		Number(Boolean(b.children?.length)) - Number(Boolean(a.children?.length))
	);
}
