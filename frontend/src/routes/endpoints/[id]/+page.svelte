<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { edgeClient, type Endpoint, type Webhook, ProviderType, WebhookStatus } from '$lib/api/client';

	let endpoint = $state<Endpoint | null>(null);
	let webhookUrl = $state<string>('');
	let webhooks = $state<Webhook[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let copiedUrl = $state(false);

	$effect(() => {
		loadEndpoint($page.params.id);
	});

	async function loadEndpoint(id: string) {
		loading = true;
		error = null;
		try {
			const [endpointResponse, webhooksResponse] = await Promise.all([
				edgeClient.getEndpoint({ id }),
				edgeClient.listWebhooks({ endpointId: id, pagination: { pageSize: 10 } })
			]);
			endpoint = endpointResponse.endpoint ?? null;
			webhookUrl = endpointResponse.webhookUrl;
			webhooks = webhooksResponse.webhooks;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to fetch endpoint';
		} finally {
			loading = false;
		}
	}

	function getProviderLabel(provider: ProviderType): string {
		switch (provider) {
			case ProviderType.STRIPE: return 'Stripe';
			case ProviderType.GITHUB: return 'GitHub';
			case ProviderType.TELEGRAM: return 'Telegram';
			case ProviderType.GENERIC: return 'Generic';
			default: return 'Unknown';
		}
	}

	function getStatusBadge(status: WebhookStatus): { class: string; label: string } {
		switch (status) {
			case WebhookStatus.PENDING: return { class: 'badge-pending', label: 'Pending' };
			case WebhookStatus.DELIVERED: return { class: 'badge-delivered', label: 'Delivered' };
			case WebhookStatus.FAILED: return { class: 'badge-failed', label: 'Failed' };
			case WebhookStatus.DEAD_LETTER: return { class: 'badge-dead-letter', label: 'Dead Letter' };
			default: return { class: '', label: 'Unknown' };
		}
	}

	function formatDate(timestamp: { seconds?: bigint }): string {
		if (!timestamp?.seconds) return 'N/A';
		return new Date(Number(timestamp.seconds) * 1000).toLocaleString();
	}

	async function copyWebhookUrl() {
		try {
			await navigator.clipboard.writeText(webhookUrl);
			copiedUrl = true;
			setTimeout(() => copiedUrl = false, 2000);
		} catch (e) {
			console.error('Failed to copy:', e);
		}
	}

	async function toggleMute() {
		if (!endpoint) return;
		try {
			await edgeClient.updateEndpoint({
				id: endpoint.id,
				muted: !endpoint.muted
			});
			await loadEndpoint(endpoint.id);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to update endpoint';
		}
	}
</script>

<div class="space-y-6">
	{#if loading}
		<div class="animate-pulse space-y-4">
			<div class="h-8 w-48 bg-[var(--color-muted)] rounded"></div>
			<div class="h-4 w-96 bg-[var(--color-muted)] rounded"></div>
		</div>
	{:else if error}
		<div class="rounded-lg border border-[var(--color-destructive)] bg-[var(--color-destructive)]/10 p-4">
			<p class="text-[var(--color-destructive)]">{error}</p>
		</div>
	{:else if endpoint}
		<div>
			<a href="/endpoints" class="text-sm text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]">
				← Back to Endpoints
			</a>
			<div class="flex items-center gap-4 mt-2">
				<h1 class="text-2xl font-bold text-[var(--color-foreground)]">{endpoint.name}</h1>
				{#if endpoint.muted}
					<span class="inline-flex items-center rounded-full px-2 py-1 text-xs font-medium bg-[var(--color-muted)] text-[var(--color-muted-foreground)]">
						Muted
					</span>
				{/if}
			</div>
			<p class="text-[var(--color-muted-foreground)]">{getProviderLabel(endpoint.providerType)} webhook endpoint</p>
		</div>

		<!-- Webhook URL Card -->
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6">
			<h2 class="text-lg font-semibold text-[var(--color-foreground)] mb-2">Webhook URL</h2>
			<p class="text-sm text-[var(--color-muted-foreground)] mb-4">
				Configure this URL in your provider's webhook settings
			</p>
			<div class="flex items-center gap-2">
				<code class="flex-1 bg-[var(--color-muted)] px-4 py-2 rounded-md font-mono text-sm overflow-x-auto">
					{webhookUrl}
				</code>
				<button
					onclick={copyWebhookUrl}
					class="px-4 py-2 rounded-md border border-[var(--color-border)] text-sm font-medium hover:bg-[var(--color-muted)] transition-colors"
				>
					{copiedUrl ? 'Copied!' : 'Copy'}
				</button>
			</div>
		</div>

		<!-- Endpoint Details -->
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6">
			<div class="flex items-center justify-between mb-4">
				<h2 class="text-lg font-semibold text-[var(--color-foreground)]">Details</h2>
				<div class="flex gap-2">
					<button
						onclick={toggleMute}
						class="px-3 py-1 rounded border border-[var(--color-border)] text-sm hover:bg-[var(--color-muted)] transition-colors"
					>
						{endpoint.muted ? 'Unmute' : 'Mute'}
					</button>
					<a
						href="/endpoints/{endpoint.id}/edit"
						class="px-3 py-1 rounded border border-[var(--color-border)] text-sm hover:bg-[var(--color-muted)] transition-colors"
					>
						Edit
					</a>
				</div>
			</div>
			<dl class="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm">
				<div>
					<dt class="text-[var(--color-muted-foreground)]">Destination URL</dt>
					<dd class="font-mono mt-1">{endpoint.destinationUrl}</dd>
				</div>
				<div>
					<dt class="text-[var(--color-muted-foreground)]">Created</dt>
					<dd class="mt-1">{formatDate(endpoint.createdAt)}</dd>
				</div>
			</dl>
		</div>

		<!-- Recent Webhooks -->
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] overflow-hidden">
			<div class="px-6 py-4 border-b border-[var(--color-border)]">
				<div class="flex items-center justify-between">
					<h2 class="text-lg font-semibold text-[var(--color-foreground)]">Recent Webhooks</h2>
					<a href="/webhooks?endpoint={endpoint.id}" class="text-sm text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]">
						View all →
					</a>
				</div>
			</div>
			{#if webhooks.length === 0}
				<div class="p-8 text-center text-[var(--color-muted-foreground)]">
					No webhooks received yet
				</div>
			{:else}
				<table class="w-full">
					<thead class="bg-[var(--color-muted)]">
						<tr>
							<th class="text-left px-4 py-2 text-xs font-medium text-[var(--color-muted-foreground)]">Received</th>
							<th class="text-left px-4 py-2 text-xs font-medium text-[var(--color-muted-foreground)]">Status</th>
							<th class="text-left px-4 py-2 text-xs font-medium text-[var(--color-muted-foreground)]">Attempts</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-[var(--color-border)]">
						{#each webhooks as webhook}
							{@const status = getStatusBadge(webhook.status)}
							<tr class="hover:bg-[var(--color-muted)]/50">
								<td class="px-4 py-2">
									<a href="/webhooks/{webhook.id}" class="text-sm hover:underline">
										{formatDate(webhook.receivedAt)}
									</a>
								</td>
								<td class="px-4 py-2">
									<span class="{status.class} inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium">
										{status.label}
									</span>
								</td>
								<td class="px-4 py-2 text-sm text-[var(--color-muted-foreground)]">
									{webhook.attempts}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</div>
	{/if}
</div>
