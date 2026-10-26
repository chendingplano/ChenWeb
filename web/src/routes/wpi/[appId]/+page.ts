import type { PageLoad } from './$types';

// Client-rendered like the rest of this app.
export const ssr = false;

export const load: PageLoad = ({ params }) => ({ appId: params.appId });
