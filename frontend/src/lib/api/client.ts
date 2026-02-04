import { createClient, Code, type Interceptor } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { EdgeService } from '$api/hookly/v1/edge_pb';

// Interceptor that redirects to login on auth failures
const authRedirectInterceptor: Interceptor = (next) => async (req) => {
	try {
		return await next(req);
	} catch (err) {
		if (err && typeof err === 'object' && 'code' in err && err.code === Code.Unauthenticated) {
			// Redirect to login page
			if (typeof window !== 'undefined') {
				window.location.href = '/auth/login';
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
