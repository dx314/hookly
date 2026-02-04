import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { EdgeService } from '$api/hookly/v1/edge_pb';

// Create transport with credentials (cookies)
const transport = createConnectTransport({
	baseUrl: '',  // Same origin
	credentials: 'same-origin',  // Send cookies
});

// Create EdgeService client
export const edgeClient = createClient(EdgeService, transport);

// Re-export types
export { type Endpoint, type Webhook, type SystemStatus } from '$api/hookly/v1/common_pb';
export { ProviderType, WebhookStatus } from '$api/hookly/v1/common_pb';
