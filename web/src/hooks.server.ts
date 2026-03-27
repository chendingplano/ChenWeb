import type { Handle } from '@sveltejs/kit';
import { redirect } from '@sveltejs/kit';
import { sequence } from '@sveltejs/kit/hooks';
import { paraglideMiddleware } from '$lib/paraglide/server';

// Go backend URL (server-side)
const APP_BASE_URL = process.env.APP_BASE_URL
if (typeof APP_BASE_URL !== "string" || APP_BASE_URL.length <= 0) {
	console.error("missing APP_BASE_URL")
}

// Routes that require authentication
const PROTECTED_ROUTES = ['/dashboard'];

/**
 * Session validation result
 */
type SessionValidationResult = {
	authenticated: boolean;
	requires2FA: boolean;
};

/**
 * Validate session via Go backend's /auth/me endpoint
 * The backend validates with Kratos internally
 */
async function validateSession(cookies: string | null): Promise<SessionValidationResult> {
	if (!cookies) return { authenticated: false, requires2FA: false };

	try {
		const response = await fetch(`${APP_BASE_URL}/auth/me`, {
			method: 'GET',
			headers: {
				'Accept': 'application/json',
				'Cookie': cookies,
			},
		});

		if (response.ok) {
			return { authenticated: true, requires2FA: false };
		}

		// Check for 403 Forbidden - this typically means AAL2 (2FA) is required
		if (response.status === 403) {
			return { authenticated: false, requires2FA: true };
		}

		return { authenticated: false, requires2FA: false };
	} catch (error) {
		console.error('Failed to validate session:', error);
		return { authenticated: false, requires2FA: false };
	}
}

/**
 * Check if a path matches any protected route prefix
 */
function isProtectedRoute(pathname: string): boolean {
	return PROTECTED_ROUTES.some(route => pathname === route || pathname.startsWith(route + '/'));
}

/**
 * Authentication middleware
 */
const handleAuth: Handle = async ({ event, resolve }) => {
	const pathname = event.url.pathname;

	// Check if this is a protected route
	if (isProtectedRoute(pathname)) {
		const cookies = event.request.headers.get('cookie');
		const result = await validateSession(cookies);

		if (!result.authenticated) {
			const returnTo = encodeURIComponent(event.url.pathname + event.url.search);

			if (result.requires2FA) {
				// User has valid AAL1 session but needs to complete 2FA
				throw redirect(302, `/verify-2fa?return_to=${returnTo}`);
			}

			// No valid session - redirect to login
			throw redirect(302, `/login?return_to=${returnTo}`);
		}
	}

	return resolve(event);
};

const handleParaglide: Handle = ({ event, resolve }) =>
	paraglideMiddleware(event.request, ({ request, locale }) => {
		event.request = request;

		return resolve(event, {
			transformPageChunk: ({ html }) => html.replace('%paraglide.lang%', locale)
		});
	});

export const handle: Handle = sequence(handleAuth, handleParaglide);
