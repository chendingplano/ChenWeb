// Jenny Gu storefront runs as a client-side SPA so it can read/write
// localStorage (reviews, patterns, favorites, cart) without SSR guards.
export const ssr = false;
export const prerender = false;
