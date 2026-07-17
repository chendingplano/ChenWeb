// Site config for the SemOS customer-facing frontend (ADR 2026071102).
// Shapes mirror server/api/sitehandler/sitehandler.go JSON tags exactly.

export interface SiteBranding {
	site_name: string;
	logo_text: string;
	logo_image: string;
	powered_by: string;
}

export interface SiteHero {
	kicker: string;
	slogan: string;
	subtitle: string;
	image: string;
	cta_primary_label: string;
	cta_primary_href: string;
	cta_secondary_label: string;
	cta_secondary_href: string;
}

/** Heading block above a page section. */
export interface SectionCopy {
	kicker: string;
	title: string;
	subtitle: string;
}

/** One figure in the stats band. */
export interface SiteStat {
	label: string;
	value: string;
}

/** One entry in a footer link column. */
export interface FooterLink {
	label: string;
	href: string;
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

/** One numbered conviction on the About page. */
export interface AboutValue {
	title: string;
	description: string;
}

/**
 * Copy for the 关于我们 (About Us) page. Contact details (email, address)
 * are not duplicated here — the page reads them from SiteFooter.
 * Nullable arrays: Go marshals nil slices to `null` (see `story`/`values` below).
 */
export interface SiteAbout {
	kicker: string;
	title: string;
	lede: string;
	image: string;
	story_kicker: string;
	story: string[] | null;
	values_kicker: string;
	values_title: string;
	contact_kicker: string;
	contact_title: string;
	contact_text: string;
	values: AboutValue[] | null;
}

export interface WorkspaceApp {
	// Stable identifier (e.g. "knowledge_base"), independent of `name`. Join
	// key for [workspace-content] visibility and labels-<lang>.toml overrides
	// (ADR 2026071701).
	key: string;
	name: string;
	description: string;
	href: string;
	icon: string;
}

export interface SiteWorkspace {
	kicker: string;
	banner_title: string;
	banner_subtitle: string;
	banner_image: string;
	apps: WorkspaceApp[];
}

export interface SiteFooter {
	text: string;
	address: string;
	newsletter: string;
	email: string;
	quick_links: FooterLink[];
	resources: FooterLink[];
}

export interface SiteConfig {
	branding: SiteBranding;
	hero: SiteHero;
	highlights: SiteHighlight[];
	features_section: SectionCopy;
	features: SiteFeature[];
	stats: SiteStat[];
	cta: SectionCopy;
	about: SiteAbout;
	footer: SiteFooter;
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

// /semos/workspace content item id -> enabled. Ids absent from the map
// default to enabled on the frontend. See [workspace-content] in
// config.local.toml (ADR 2026071701).
export type WorkspaceContentVisibility = Record<string, boolean>;

// /semos/workspace content item id -> label override for the requested
// language. Ids absent from the map keep their site-config default label.
// See config/workspace-content/labels-<lang>.toml (ADR 2026071701).
export type WorkspaceContentLabels = Record<string, string>;

// /semos/workspace app tile id (app.key) -> description override for the
// requested language. Ids absent from the map keep their site-config default
// description. See config/workspace-content/labels-<lang>.toml's
// [descriptions] table (ADR 2026071701).
export type WorkspaceContentDescriptions = Record<string, string>;

/** Workspace content visibility + i18n label/description overrides — authenticated. */
export async function getWorkspaceContentConfig(
	lang?: string,
	fetchFn: typeof fetch = fetch
): Promise<{
	visibility: WorkspaceContentVisibility;
	labels: WorkspaceContentLabels;
	descriptions: WorkspaceContentDescriptions;
}> {
	const query = lang ? `?lang=${encodeURIComponent(lang)}` : '';
	const response = await getJSON<{
		status: boolean;
		visibility: WorkspaceContentVisibility;
		labels: WorkspaceContentLabels;
		descriptions: WorkspaceContentDescriptions;
	}>(`/api/v1/workspace/content-config${query}`, fetchFn);
	return { visibility: response.visibility, labels: response.labels, descriptions: response.descriptions };
}
