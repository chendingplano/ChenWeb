// Client for the DB-backed, language-aware page content configuration API
// (kb.page_def / kb.page_config; KnowledgeStore spec 2026072001 §9).
//
// The resolution endpoint returns only the entries that are enabled AND
// authorized for the current user, each with content resolved for the requested
// locale. Callers treat presence in the returned map as "visible", and use the
// per-entry `label`/`description` as overrides (falling back to the page's own
// hardcoded defaults when a field is absent).

export type PageEntryContent = {
	label?: string;
	description?: string;
};

// entry_key -> resolved content override for the requested locale.
export type PageConfigMap = Record<string, PageEntryContent>;

// Overlay model (spec 2026072001 §9.4): the page owns its structure and
// hardcoded defaults; config only overrides text, hides, or restricts items.
// - `overrides`: entry_key -> resolved content for entries that are visible.
// - `hidden`: entry_keys that have a config row but must be hidden (disabled,
//   suspended, or unauthorized for this user).
// An entry_key in NEITHER has no config row, so the page renders its own
// hardcoded default (this is what a deleted entry falls back to).
export type PageConfig = {
	overrides: PageConfigMap;
	hidden: Set<string>;
};

type PageConfigResponse = {
	status: boolean;
	page_key: string;
	lang: string;
	entries: Array<{ entry_key: string; content: PageEntryContent }>;
	hidden: string[];
};

/**
 * Resolve a page's configurable content for the current user and locale.
 * Authenticated (same-origin cookies). Throws on non-2xx so callers can fail
 * open to their built-in defaults.
 */
export async function getPageConfig(
	pageKey: string,
	lang?: string,
	fetchFn: typeof fetch = fetch
): Promise<PageConfig> {
	const query = lang ? `?lang=${encodeURIComponent(lang)}` : '';
	const res = await fetchFn(`/api/v1/page-config/${encodeURIComponent(pageKey)}${query}`, {
		credentials: 'same-origin'
	});
	if (!res.ok) {
		throw new Error(`page-config request failed: ${res.status} ${pageKey}`);
	}
	const data = (await res.json()) as PageConfigResponse;
	const overrides: PageConfigMap = {};
	for (const entry of data.entries ?? []) {
		overrides[entry.entry_key] = entry.content ?? {};
	}
	return { overrides, hidden: new Set(data.hidden ?? []) };
}

// ── Admin ────────────────────────────────────────────────────────────────

export type PageDef = {
	page_key: string;
	route: string;
	title: string;
	description: string;
};

export type AdminEntry = {
	entry_key: string;
	entry_desc: string;
	content: Record<string, PageEntryContent>;
	access_role: string[];
	accessible: boolean;
	enabled: boolean;
};

export async function listPages(fetchFn: typeof fetch = fetch): Promise<PageDef[]> {
	const res = await fetchFn('/api/v1/page-config/admin/pages', { credentials: 'same-origin' });
	if (!res.ok) throw new Error(`list pages failed: ${res.status}`);
	const data = (await res.json()) as { status: boolean; pages: PageDef[] };
	return data.pages ?? [];
}

export async function listEntries(
	pageKey: string,
	fetchFn: typeof fetch = fetch
): Promise<AdminEntry[]> {
	const res = await fetchFn(
		`/api/v1/page-config/admin/pages/${encodeURIComponent(pageKey)}/entries`,
		{ credentials: 'same-origin' }
	);
	if (!res.ok) throw new Error(`list entries failed: ${res.status}`);
	const data = (await res.json()) as { status: boolean; entries: AdminEntry[] };
	return data.entries ?? [];
}

export async function upsertEntry(
	pageKey: string,
	entry: AdminEntry,
	fetchFn: typeof fetch = fetch
): Promise<void> {
	const res = await fetchFn(
		`/api/v1/page-config/admin/pages/${encodeURIComponent(pageKey)}/entries/${encodeURIComponent(entry.entry_key)}`,
		{
			method: 'PUT',
			credentials: 'same-origin',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(entry)
		}
	);
	if (!res.ok) throw new Error(`save entry failed: ${res.status}`);
}

export async function deleteEntry(
	pageKey: string,
	entryKey: string,
	fetchFn: typeof fetch = fetch
): Promise<void> {
	const res = await fetchFn(
		`/api/v1/page-config/admin/pages/${encodeURIComponent(pageKey)}/entries/${encodeURIComponent(entryKey)}`,
		{ method: 'DELETE', credentials: 'same-origin' }
	);
	if (!res.ok) throw new Error(`delete entry failed: ${res.status}`);
}
