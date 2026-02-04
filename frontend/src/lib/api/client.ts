import { createClient, Code, ConnectError, type Interceptor } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { EdgeService } from '$api/hookly/v1/edge_pb';

// Interceptor that redirects to login on auth failures
const authRedirectInterceptor: Interceptor = (next) => async (req) => {
	try {
		return await next(req);
	} catch (err) {
		if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
			// Redirect to login page - don't throw, just redirect and hang
			if (typeof window !== 'undefined') {
				window.location.href = '/login';
				// Return a promise that never resolves to prevent error display during redirect
				return new Promise(() => {});
			}
		}
		throw err;
	}
};

// Custom fetch that includes credentials
const fetchWithCredentials: typeof globalThis.fetch = (input, init) => {
	return globalThis.fetch(input, {
		...init,
		credentials: 'same-origin',
	});
};

// Create transport with credentials (cookies)
const transport = createConnectTransport({
	baseUrl: '',  // Same origin
	fetch: fetchWithCredentials,
	interceptors: [authRedirectInterceptor],
});

// Create EdgeService client
export const edgeClient = createClient(EdgeService, transport);

// Re-export types
export { type Endpoint, type Webhook, type SystemStatus } from '$api/hookly/v1/common_pb';
export { ProviderType, WebhookStatus } from '$api/hookly/v1/common_pb';
