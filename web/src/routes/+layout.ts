import { db_store } from '@chendingplano/shared';

// Initialize the shared db_store with VITE_APP_DOMAIN_NAME domain
// Use import.meta.env for Vite environment variables (available in browser)
const domain = import.meta.env.VITE_APP_DOMAIN_NAME;

if (domain) {
	db_store.setDomainName(domain);
} else {
	console.error('VITE_APP_DOMAIN_NAME is not set in environment variables');
}

// Export an empty load function to make this a valid +layout.ts
export const load = () => {
	return {};
};
