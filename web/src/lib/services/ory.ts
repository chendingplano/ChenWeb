/**
 * Ory Kratos Service - Handles authentication flows with Ory Kratos
 *
 * This service provides methods for:
 * - Login (email/password, username/password, Google OAuth)
 * - Registration (email, username)
 * - Password recovery
 * - Session management
 */

// Configuration - adjust these URLs based on your environment
// Use relative path to go through Vite proxy (avoids SameSite cookie issues)
const KRATOS_PUBLIC_URL = '/kratos';
const KRATOS_UI_URL = 'http://127.0.0.1:4455';

// Kratos error IDs
export const OryErrorIds = {
    SESSION_ALREADY_AVAILABLE: 'session_already_available',
} as const;

export interface OryError {
    id: string;
    text: string;
    type: string;
}

export interface OryFlowUI {
    action: string;
    method: string;
    nodes: OryUINode[];
    messages?: OryError[];
}

export interface OryUINode {
    type: string;
    group: string;
    attributes: {
        name?: string;
        type?: string;
        value?: string;
        disabled?: boolean;
        node_type?: string;
        required?: boolean;
    };
    messages?: OryError[];
    meta?: {
        label?: {
            id: number;
            text: string;
            type: string;
        };
    };
}

export interface OryLoginFlow {
    id: string;
    type: string;
    expires_at: string;
    issued_at: string;
    request_url: string;
    ui: OryFlowUI;
    created_at: string;
    updated_at: string;
    refresh: boolean;
    requested_aal: string;
}

export interface OryRegistrationFlow {
    id: string;
    type: string;
    expires_at: string;
    issued_at: string;
    request_url: string;
    ui: OryFlowUI;
}

export interface OryRecoveryFlow {
    id: string;
    type: string;
    expires_at: string;
    issued_at: string;
    request_url: string;
    ui: OryFlowUI;
    state: string;
}

export interface OrySession {
    id: string;
    active: boolean;
    expires_at: string;
    authenticated_at: string;
    authenticator_assurance_level: string;
    authentication_methods: Array<{
        method: string;
        aal: string;
        completed_at: string;
    }>;
    issued_at: string;
    identity: OryIdentity;
}

export interface OryIdentity {
    id: string;
    schema_id: string;
    schema_url: string;
    state: string;
    state_changed_at: string;
    traits: {
        email?: string;
        username?: string;
        name?: {
            first?: string;
            last?: string;
        };
    };
    verifiable_addresses?: Array<{
        id: string;
        value: string;
        verified: boolean;
        via: string;
        status: string;
    }>;
    recovery_addresses?: Array<{
        id: string;
        value: string;
        via: string;
    }>;
    metadata_public?: Record<string, unknown>;
    created_at: string;
    updated_at: string;
}

export interface OryServiceResult<T> {
    success: boolean;
    data?: T;
    error?: string;
    errorId?: string; // Kratos error ID (e.g., 'session_already_available')
    redirectTo?: string;
}

/**
 * Create a new login flow
 *
 * @param returnTo - Optional URL to redirect to after login
 * @param refresh - If true, refreshes the session instead of creating a new login flow.
 *                  Use this when the user is already logged in and you want to re-authenticate.
 */
export async function createLoginFlow(returnTo?: string, refresh?: boolean): Promise<OryServiceResult<OryLoginFlow>> {
    try {
        const params = new URLSearchParams();
        if (returnTo) {
            params.set('return_to', returnTo);
        }
        if (refresh) {
            params.set('refresh', 'true');
        }

        const response = await fetch(`${KRATOS_PUBLIC_URL}/self-service/login/browser?${params}`, {
            method: 'GET',
            credentials: 'include',
            headers: {
                'Accept': 'application/json',
            },
        });

        if (!response.ok) {
            const errorData = await response.json().catch(() => ({}));
            const errorId = errorData.error?.id;
            const errorMessage = errorData.error?.message || `Failed to create login flow: ${response.status}`;

            return {
                success: false,
                error: errorMessage,
                errorId: errorId,
            };
        }

        const data = await response.json();
        return { success: true, data };
    } catch (error) {
        return {
            success: false,
            error: error instanceof Error ? error.message : 'Network error creating login flow',
        };
    }
}

/**
 * Submit login credentials (password method)
 */
export async function submitLogin(
    flowId: string,
    identifier: string,
    password: string,
    csrfToken: string
): Promise<OryServiceResult<OrySession>> {
    try {
        const response = await fetch(`${KRATOS_PUBLIC_URL}/self-service/login?flow=${flowId}`, {
            method: 'POST',
            credentials: 'include',
            headers: {
                'Accept': 'application/json',
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                method: 'password',
                identifier,
                password,
                csrf_token: csrfToken,
            }),
        });

        // Handle redirect (successful login)
        if (response.redirected) {
            return {
                success: true,
                redirectTo: response.url,
            };
        }

        const data = await response.json();

        if (!response.ok) {
            // Check for validation errors in the UI
            const errorMessages = extractErrors(data.ui);
            return {
                success: false,
                error: errorMessages || data.error?.message || 'Login failed',
            };
        }

        return { success: true, data };
    } catch (error) {
        return {
            success: false,
            error: error instanceof Error ? error.message : 'Network error during login',
        };
    }
}

/**
 * Create a new registration flow
 */
export async function createRegistrationFlow(returnTo?: string): Promise<OryServiceResult<OryRegistrationFlow>> {
    try {
        const params = new URLSearchParams();
        if (returnTo) {
            params.set('return_to', returnTo);
        }

        const response = await fetch(`${KRATOS_PUBLIC_URL}/self-service/registration/browser?${params}`, {
            method: 'GET',
            credentials: 'include',
            headers: {
                'Accept': 'application/json',
            },
        });

        if (!response.ok) {
            const errorData = await response.json().catch(() => ({}));
            const errorId = errorData.error?.id;
            const errorMessage = errorData.error?.message || `Failed to create registration flow: ${response.status}`;

            return {
                success: false,
                error: errorMessage,
                errorId: errorId,
            };
        }

        const data = await response.json();
        return { success: true, data };
    } catch (error) {
        return {
            success: false,
            error: error instanceof Error ? error.message : 'Network error creating registration flow',
        };
    }
}

/**
 * Submit registration with email
 */
export async function submitEmailRegistration(
    flowId: string,
    email: string,
    password: string,
    firstName: string,
    lastName: string,
    csrfToken: string
): Promise<OryServiceResult<OrySession>> {
    try {
        const response = await fetch(`${KRATOS_PUBLIC_URL}/self-service/registration?flow=${flowId}`, {
            method: 'POST',
            credentials: 'include',
            headers: {
                'Accept': 'application/json',
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                method: 'password',
                password,
                csrf_token: csrfToken,
                traits: {
                    email,
                    name: {
                        first: firstName,
                        last: lastName,
                    },
                },
            }),
        });

        if (response.redirected) {
            return {
                success: true,
                redirectTo: response.url,
            };
        }

        const data = await response.json();

        if (!response.ok) {
            const errorMessages = extractErrors(data.ui);
            return {
                success: false,
                error: errorMessages || data.error?.message || 'Registration failed',
            };
        }

        return { success: true, data };
    } catch (error) {
        return {
            success: false,
            error: error instanceof Error ? error.message : 'Network error during registration',
        };
    }
}

/**
 * Submit registration with username
 */
export async function submitUsernameRegistration(
    flowId: string,
    username: string,
    password: string,
    firstName?: string,
    lastName?: string,
    csrfToken?: string
): Promise<OryServiceResult<OrySession>> {
    try {
        const traits: Record<string, unknown> = { username };
        if (firstName || lastName) {
            traits.name = {
                first: firstName || '',
                last: lastName || '',
            };
        }

        const response = await fetch(`${KRATOS_PUBLIC_URL}/self-service/registration?flow=${flowId}`, {
            method: 'POST',
            credentials: 'include',
            headers: {
                'Accept': 'application/json',
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                method: 'password',
                password,
                csrf_token: csrfToken,
                traits,
            }),
        });

        if (response.redirected) {
            return {
                success: true,
                redirectTo: response.url,
            };
        }

        const data = await response.json();

        if (!response.ok) {
            const errorMessages = extractErrors(data.ui);
            return {
                success: false,
                error: errorMessages || data.error?.message || 'Registration failed',
            };
        }

        return { success: true, data };
    } catch (error) {
        return {
            success: false,
            error: error instanceof Error ? error.message : 'Network error during registration',
        };
    }
}

/**
 * Create a recovery (forgot password) flow
 */
export async function createRecoveryFlow(returnTo?: string): Promise<OryServiceResult<OryRecoveryFlow>> {
    try {
        const params = new URLSearchParams();
        if (returnTo) {
            params.set('return_to', returnTo);
        }

        const response = await fetch(`${KRATOS_PUBLIC_URL}/self-service/recovery/browser?${params}`, {
            method: 'GET',
            credentials: 'include',
            headers: {
                'Accept': 'application/json',
            },
        });

        if (!response.ok) {
            const errorData = await response.json().catch(() => ({}));
            return {
                success: false,
                error: errorData.error?.message || `Failed to create recovery flow: ${response.status}`,
            };
        }

        const data = await response.json();
        return { success: true, data };
    } catch (error) {
        return {
            success: false,
            error: error instanceof Error ? error.message : 'Network error creating recovery flow',
        };
    }
}

/**
 * Submit recovery email
 */
export async function submitRecoveryEmail(
    flowId: string,
    email: string,
    csrfToken: string
): Promise<OryServiceResult<OryRecoveryFlow>> {
    try {
        const response = await fetch(`${KRATOS_PUBLIC_URL}/self-service/recovery?flow=${flowId}`, {
            method: 'POST',
            credentials: 'include',
            headers: {
                'Accept': 'application/json',
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                method: 'code',
                email,
                csrf_token: csrfToken,
            }),
        });

        const data = await response.json();

        if (!response.ok) {
            const errorMessages = extractErrors(data.ui);
            return {
                success: false,
                error: errorMessages || data.error?.message || 'Recovery request failed',
            };
        }

        return { success: true, data };
    } catch (error) {
        return {
            success: false,
            error: error instanceof Error ? error.message : 'Network error during recovery',
        };
    }
}

/**
 * Get the current session
 */
export async function getSession(): Promise<OryServiceResult<OrySession>> {
    try {
        const response = await fetch(`${KRATOS_PUBLIC_URL}/sessions/whoami`, {
            method: 'GET',
            credentials: 'include',
            headers: {
                'Accept': 'application/json',
            },
        });

        if (response.status === 401) {
            return {
                success: false,
                error: 'Not authenticated',
            };
        }

        if (!response.ok) {
            const errorData = await response.json().catch(() => ({}));
            return {
                success: false,
                error: errorData.error?.message || `Failed to get session: ${response.status}`,
            };
        }

        const data = await response.json();
        return { success: true, data };
    } catch (error) {
        return {
            success: false,
            error: error instanceof Error ? error.message : 'Network error getting session',
        };
    }
}

/**
 * Logout - creates a logout flow and redirects
 */
export async function logout(): Promise<OryServiceResult<{ logout_url: string }>> {
    try {
        const response = await fetch(`${KRATOS_PUBLIC_URL}/self-service/logout/browser`, {
            method: 'GET',
            credentials: 'include',
            headers: {
                'Accept': 'application/json',
            },
        });

        if (!response.ok) {
            const errorData = await response.json().catch(() => ({}));
            return {
                success: false,
                error: errorData.error?.message || `Failed to create logout flow: ${response.status}`,
            };
        }

        const data = await response.json();
        return { success: true, data };
    } catch (error) {
        return {
            success: false,
            error: error instanceof Error ? error.message : 'Network error during logout',
        };
    }
}

/**
 * Get Google OAuth login URL
 * This initiates the OIDC flow with Google through Kratos
 */
export async function getGoogleLoginUrl(flowId: string): Promise<string> {
    return `${KRATOS_PUBLIC_URL}/self-service/login?flow=${flowId}`;
}

/**
 * Submit Google OIDC login
 * Redirects to Google OAuth flow
 */
export async function initiateGoogleLogin(flowId: string, csrfToken: string): Promise<OryServiceResult<void>> {
    try {
        // For OIDC providers, we need to submit a form that triggers the redirect
        const form = document.createElement('form');
        form.method = 'POST';
        form.action = `${KRATOS_PUBLIC_URL}/self-service/login?flow=${flowId}`;

        const methodInput = document.createElement('input');
        methodInput.type = 'hidden';
        methodInput.name = 'method';
        methodInput.value = 'oidc';
        form.appendChild(methodInput);

        const providerInput = document.createElement('input');
        providerInput.type = 'hidden';
        providerInput.name = 'provider';
        providerInput.value = 'google';
        form.appendChild(providerInput);

        const csrfInput = document.createElement('input');
        csrfInput.type = 'hidden';
        csrfInput.name = 'csrf_token';
        csrfInput.value = csrfToken;
        form.appendChild(csrfInput);

        document.body.appendChild(form);
        form.submit();

        return { success: true };
    } catch (error) {
        return {
            success: false,
            error: error instanceof Error ? error.message : 'Failed to initiate Google login',
        };
    }
}

/**
 * Helper function to extract the CSRF token from a flow
 */
export function extractCsrfToken(ui: OryFlowUI): string | undefined {
    const csrfNode = ui.nodes.find(
        node => node.attributes.name === 'csrf_token'
    );
    return csrfNode?.attributes.value;
}

/**
 * Helper function to extract error messages from UI
 */
function extractErrors(ui?: OryFlowUI): string | undefined {
    if (!ui) return undefined;

    const errors: string[] = [];

    // Check for flow-level messages
    if (ui.messages) {
        errors.push(...ui.messages.filter(m => m.type === 'error').map(m => m.text));
    }

    // Check for node-level messages
    for (const node of ui.nodes) {
        if (node.messages) {
            errors.push(...node.messages.filter(m => m.type === 'error').map(m => m.text));
        }
    }

    return errors.length > 0 ? errors.join('. ') : undefined;
}

/**
 * Check if an OIDC provider (like Google) is available in the flow
 */
export function hasOidcProvider(ui: OryFlowUI, provider: string): boolean {
    return ui.nodes.some(
        node =>
            node.group === 'oidc' &&
            node.attributes.value === provider
    );
}

export { KRATOS_PUBLIC_URL, KRATOS_UI_URL };
