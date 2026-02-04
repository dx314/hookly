<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { edgeClient, type Webhook, type Endpoint, WebhookStatus } from '$lib/api/client';

	let webhooks = $state<Webhook[]>([]);
	let endpoints = $state<Endpoint[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	let selectedEndpoint = $state<string | undefined>(undefined);
	let selectedStatus = $state<WebhookStatus | undefined>(undefined);

	const statusOptions = [
		{ value: undefined, label: 'All Statuses' },
		{ value: WebhookStatus.PENDING, label: 'Pending' },
		{ value: WebhookStatus.DELIVERED, label: 'Delivered' },
		{ value: WebhookStatus.FAILED, label: 'Failed' },
		{ value: WebhookStatus.DEAD_LETTER, label: 'Dead Letter' }
	];

	onMount(async () => {
		// Check URL params
		const urlEndpoint = $page.url.searchParams.get('endpoint');
		if (urlEndpoint) {
			selectedEndpoint = urlEndpoint;
		}

		await Promise.all([loadEndpoints(), loadWebhooks()]);
	});

	$effect(() => {
		loadWebhooks();
	});

	async function loadEndpoints() {
		try {
			const response = await edgeClient.listEndpoints({});
			endpoints = response.endpoints;
		} catch (e) {
			console.error('Failed to load endpoints:', e);
		}
	}

	async function loadWebhooks() {
		loading = true;
		error = null;
		try {
			const response = await edgeClient.listWebhooks({
				endpointId: selectedEndpoint,
				status: selectedStatus,
				pagination: { pageSize: 50 }
			});
			webhooks = response.webhooks;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to fetch webhooks';
		} finally {
			loading = false;
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

	function formatDate(timestamp: { seconds?: bigint } | undefined): string {
		if (!timestamp?.seconds) return 'N/A';
		return new Date(Number(timestamp.seconds) * 1000).toLocaleString();
	}

	function getEndpointName(endpointId: string): string {
		const endpoint = endpoints.find(e => e.id === endpointId);
		return endpoint?.name ?? endpointId;
	}
</script>

<div class="space-y-6">
	<div>
		<h1 class="text-2xl font-bold text-[var(--color-foreground)]">Webhooks</h1>
		<p class="text-[var(--color-muted-foreground)]">Browse and manage received webhooks</p>
	</div>

	<!-- Filters -->
	<div class="flex gap-4">
		<select
			bind:value={selectedEndpoint}
			onchange={() => loadWebhooks()}
			class="px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)]"
		>
			<option value={undefined}>All Endpoints</option>
			{#each endpoints as endpoint (endpoint.id)}
				<option value={endpoint.id}>{endpoint.name}</option>
			{/each}
		</select>

		<select
			bind:value={selectedStatus}
			onchange={() => loadWebhooks()}
			class="px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)]"
		>
			{#each statusOptions as option (option.label)}
				<option value={option.value}>{option.label}</option>
			{/each}
		</select>
	</div>

	{#if loading}
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)]">
			<div class="p-8 text-center text-[var(--color-muted-foreground)]">Loading...</div>
		</div>
	{:else if error}
		<div class="rounded-lg border border-[var(--color-destructive)] bg-[var(--color-destructive)]/10 p-4">
			<p class="text-[var(--color-destructive)]">{error}</p>
		</div>
	{:else if webhooks.length === 0}
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-8 text-center">
			<p class="text-[var(--color-muted-foreground)]">No webhooks found.</p>
		</div>
	{:else}
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] overflow-hidden">
			<table class="w-full">
				<thead class="bg-[var(--color-muted)]">
					<tr>
						<th class="text-left px-4 py-3 text-sm font-medium text-[var(--color-muted-foreground)]">Received</th>
						<th class="text-left px-4 py-3 text-sm font-medium text-[var(--color-muted-foreground)]">Endpoint</th>
						<th class="text-left px-4 py-3 text-sm font-medium text-[var(--color-muted-foreground)]">Status</th>
						<th class="text-left px-4 py-3 text-sm font-medium text-[var(--color-muted-foreground)]">Attempts</th>
						<th class="text-left px-4 py-3 text-sm font-medium text-[var(--color-muted-foreground)]">Signature</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-[var(--color-border)]">
					{#each webhooks as webhook (webhook.id)}
						{@const status = getStatusBadge(webhook.status)}
						<tr class="hover:bg-[var(--color-muted)]/50">
							<td class="px-4 py-3">
								<a href="/webhooks/{webhook.id}" class="text-sm font-medium text-[var(--color-foreground)] hover:underline">
									{formatDate(webhook.receivedAt)}
								</a>
							</td>
							<td class="px-4 py-3">
								<a href="/endpoints/{webhook.endpointId}" class="text-sm text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]">
									{getEndpointName(webhook.endpointId)}
								</a>
							</td>
							<td class="px-4 py-3">
								<span class="{status.class} inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium">
									{status.label}
								</span>
							</td>
							<td class="px-4 py-3 text-sm text-[var(--color-muted-foreground)]">
								{webhook.attempts}
							</td>
							<td class="px-4 py-3">
								{#if webhook.signatureValid}
									<span class="text-green-600 text-sm">✓ Valid</span>
								{:else}
									<span class="text-[var(--color-muted-foreground)] text-sm">—</span>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
