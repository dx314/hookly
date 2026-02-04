<script lang="ts">
	import { onMount } from 'svelte';
	import { edgeClient, type Endpoint, ProviderType } from '$lib/api/client';

	let endpoints = $state<Endpoint[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let copiedId = $state<string | null>(null);

	onMount(async () => {
		await loadEndpoints();
	});

	async function loadEndpoints() {
		loading = true;
		error = null;
		try {
			const response = await edgeClient.listEndpoints({});
			endpoints = response.endpoints;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to fetch endpoints';
		} finally {
			loading = false;
		}
	}

	function getProviderLabel(provider: ProviderType): string {
		switch (provider) {
			case ProviderType.STRIPE:
				return 'Stripe';
			case ProviderType.GITHUB:
				return 'GitHub';
			case ProviderType.TELEGRAM:
				return 'Telegram';
			case ProviderType.GENERIC:
				return 'Generic';
			default:
				return 'Unknown';
		}
	}

	async function copyWebhookUrl(endpointId: string, webhookUrl: string) {
		try {
			await navigator.clipboard.writeText(webhookUrl);
			copiedId = endpointId;
			setTimeout(() => {
				copiedId = null;
			}, 2000);
		} catch (e) {
			console.error('Failed to copy:', e);
		}
	}

	async function toggleMute(endpoint: Endpoint) {
		try {
			await edgeClient.updateEndpoint({
				id: endpoint.id,
				muted: !endpoint.muted
			});
			await loadEndpoints();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to update endpoint';
		}
	}

	async function deleteEndpoint(id: string) {
		if (!confirm('Are you sure you want to delete this endpoint? All associated webhooks will also be deleted.')) {
			return;
		}
		try {
			await edgeClient.deleteEndpoint({ id });
			await loadEndpoints();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to delete endpoint';
		}
	}
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold text-[var(--color-foreground)]">Endpoints</h1>
			<p class="text-[var(--color-muted-foreground)]">Manage your webhook endpoints</p>
		</div>
		<a
			href="/endpoints/new"
			class="inline-flex items-center justify-center rounded-md bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-foreground)] hover:bg-[var(--color-primary)]/90 transition-colors"
		>
			Create Endpoint
		</a>
	</div>

	{#if loading}
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)]">
			<div class="p-8 text-center text-[var(--color-muted-foreground)]">Loading...</div>
		</div>
	{:else if error}
		<div class="rounded-lg border border-[var(--color-destructive)] bg-[var(--color-destructive)]/10 p-4">
			<p class="text-[var(--color-destructive)]">{error}</p>
		</div>
	{:else if endpoints.length === 0}
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-8 text-center">
			<p class="text-[var(--color-muted-foreground)]">No endpoints yet.</p>
			<a
				href="/endpoints/new"
				class="inline-flex items-center justify-center rounded-md bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-foreground)] hover:bg-[var(--color-primary)]/90 transition-colors mt-4"
			>
				Create your first endpoint
			</a>
		</div>
	{:else}
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] overflow-hidden">
			<table class="w-full">
				<thead class="bg-[var(--color-muted)]">
					<tr>
						<th class="text-left px-4 py-3 text-sm font-medium text-[var(--color-muted-foreground)]">Name</th>
						<th class="text-left px-4 py-3 text-sm font-medium text-[var(--color-muted-foreground)]">Provider</th>
						<th class="text-left px-4 py-3 text-sm font-medium text-[var(--color-muted-foreground)]">Webhook URL</th>
						<th class="text-left px-4 py-3 text-sm font-medium text-[var(--color-muted-foreground)]">Status</th>
						<th class="text-right px-4 py-3 text-sm font-medium text-[var(--color-muted-foreground)]">Actions</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-[var(--color-border)]">
					{#each endpoints as endpoint (endpoint.id)}
						<tr class="hover:bg-[var(--color-muted)]/50">
							<td class="px-4 py-3">
								<a href="/endpoints/{endpoint.id}" class="text-sm font-medium text-[var(--color-foreground)] hover:underline">
									{endpoint.name}
								</a>
							</td>
							<td class="px-4 py-3">
								<span class="text-sm text-[var(--color-muted-foreground)]">
									{getProviderLabel(endpoint.providerType)}
								</span>
							</td>
							<td class="px-4 py-3">
								<div class="flex items-center gap-2">
									<code class="text-xs bg-[var(--color-muted)] px-2 py-1 rounded font-mono truncate max-w-[200px]">
										/h/{endpoint.id}
									</code>
									<button
										onclick={() => copyWebhookUrl(endpoint.id, `/h/${endpoint.id}`)}
										class="text-xs text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)] transition-colors"
									>
										{copiedId === endpoint.id ? '✓' : 'Copy'}
									</button>
								</div>
							</td>
							<td class="px-4 py-3">
								{#if endpoint.muted}
									<span class="inline-flex items-center rounded-full px-2 py-1 text-xs font-medium bg-[var(--color-muted)] text-[var(--color-muted-foreground)]">
										Muted
									</span>
								{:else}
									<span class="inline-flex items-center rounded-full px-2 py-1 text-xs font-medium bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400">
										Active
									</span>
								{/if}
							</td>
							<td class="px-4 py-3 text-right">
								<div class="flex items-center justify-end gap-2">
									<button
										onclick={() => toggleMute(endpoint)}
										class="text-xs px-2 py-1 rounded border border-[var(--color-border)] text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)] hover:border-[var(--color-foreground)] transition-colors"
									>
										{endpoint.muted ? 'Unmute' : 'Mute'}
									</button>
									<a
										href="/endpoints/{endpoint.id}/edit"
										class="text-xs px-2 py-1 rounded border border-[var(--color-border)] text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)] hover:border-[var(--color-foreground)] transition-colors"
									>
										Edit
									</a>
									<button
										onclick={() => deleteEndpoint(endpoint.id)}
										class="text-xs px-2 py-1 rounded border border-[var(--color-destructive)] text-[var(--color-destructive)] hover:bg-[var(--color-destructive)] hover:text-white transition-colors"
									>
										Delete
									</button>
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
