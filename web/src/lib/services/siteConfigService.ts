// Site config for the SemOS customer-facing frontend (ADR 2026071102).
// Shapes mirror server/api/sitehandler/sitehandler.go JSON tags exactly.

export interface SiteBranding {
	site_name: string;
	logo_text: string;
	powered_by: string;
}

export interface SiteHero {
	slogan: string;
	subtitle: string;
	image: string;
	cta_primary_label: string;
	cta_primary_href: string;
	cta_secondary_label: string;
	cta_secondary_href: string;
}

export interface SiteHighlight {
	title: string;
	description: string;
	image: string;
}

export interface SiteFeature {
	key: string;
	title: string;
	description: string;
	href: string;
}

export interface WorkspaceApp {
	name: string;
	description: string;
	href: string;
	icon: string;
}

export interface SiteWorkspace {
	banner_title: string;
	banner_subtitle: string;
	banner_image: string;
	apps: WorkspaceApp[];
}

export interface SiteConfig {
	branding: SiteBranding;
	hero: SiteHero;
	highlights: SiteHighlight[];
	features: SiteFeature[];
	footer: { text: string };
	workspace: SiteWorkspace;
}

async function getJSON<T>(url: string, fetchFn: typeof fetch): Promise<T> {
	const res = await fetchFn(url, { credentials: 'same-origin' });
	if (!res.ok) {
		throw new Error(`site config request failed: ${res.status} ${url}`);
	}
	return (await res.json()) as T;
}

/** Tenant-independent config — all public pages. */
export function fetchSiteConfig(fetchFn: typeof fetch = fetch): Promise<SiteConfig> {
	return getJSON<SiteConfig>('/api/site-config', fetchFn);
}

/** Tenant-dependent config — authenticated pages only. */
export function fetchTenantSiteConfig(
	tenantId: string,
	fetchFn: typeof fetch = fetch
): Promise<SiteConfig> {
	return getJSON<SiteConfig>(
		`/api/v1/site-config/tenant/${encodeURIComponent(tenantId)}`,
		fetchFn
	);
}
