import type { LayoutLoad } from './$types';
import { fetchSiteConfig } from '$lib/services/siteConfigService';

// Client-rendered like the rest of this app; SSR/SEO is a later optimization.
export const ssr = false;

export const load: LayoutLoad = async ({ fetch }) => {
	return { siteConfig: await fetchSiteConfig(fetch) };
};
