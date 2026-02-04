<script lang="ts">
	import { onMount } from 'svelte';
	import { edgeClient } from '$lib/api/client';
	import type { ConnectedEndpoint } from '$api/hookly/v1/common_pb';

	let status = $state<{
		pendingCount: number;
		failedCount: number;
		deadLetterCount: number;
		connectedEndpoints: ConnectedEndpoint[];
	} | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	onMount(async () => {
		try {
			const response = await edgeClient.getStatus({});
			status = {
				pendingCount: response.status?.pendingCount ?? 0,
				failedCount: response.status?.failedCount ?? 0,
				deadLetterCount: response.status?.deadLetterCount ?? 0,
				connectedEndpoints: response.status?.connectedEndpoints ?? []
			};
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to fetch status';
		} finally {
			loading = false;
		}
	});
</script>

<div class="space-y-6">
	<div>
		<h1 class="text-2xl font-bold text-[var(--color-foreground)]">Dashboard</h1>
		<p class="text-[var(--color-muted-foreground)]">Monitor your webhook relay system</p>
	</div>

	{#if loading}
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
			{#each [1, 2, 3, 4] as _}
				<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6 animate-pulse">
					<div class="h-4 w-20 bg-[var(--color-muted)] rounded mb-2"></div>
					<div class="h-8 w-12 bg-[var(--color-muted)] rounded"></div>
				</div>
			{/each}
		</div>
	{:else if error}
		<div class="rounded-lg border border-[var(--color-destructive)] bg-[var(--color-destructive)]/10 p-4">
			<p class="text-[var(--color-destructive)]">{error}</p>
		</div>
	{:else if status}
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
			<!-- Connected Relays -->
			<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6">
				<div class="flex items-center justify-between">
					<p class="text-sm font-medium text-[var(--color-muted-foreground)]">Relays</p>
					<span class="flex h-3 w-3 rounded-full {status.connectedEndpoints.length > 0 ? 'bg-green-500' : 'bg-zinc-500'}"></span>
				</div>
				<p class="text-2xl font-bold text-[var(--color-foreground)] mt-2">
					{status.connectedEndpoints.length} online
				</p>
				{#if status.connectedEndpoints.length > 0}
					<div class="mt-2 flex flex-wrap gap-1">
						{#each status.connectedEndpoints as endpoint}
							<a
								href="/endpoints/{endpoint.id}"
								class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-500/10 text-green-600 hover:bg-green-500/20 transition-colors"
							>
								{endpoint.name}
							</a>
						{/each}
					</div>
				{:else}
					<p class="text-xs text-[var(--color-muted-foreground)] mt-1">no endpoints connected</p>
				{/if}
			</div>

			<!-- Pending Webhooks -->
			<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6">
				<p class="text-sm font-medium text-[var(--color-muted-foreground)]">Pending</p>
				<p class="text-2xl font-bold text-[var(--color-status-pending)] mt-2">{status.pendingCount}</p>
				<p class="text-xs text-[var(--color-muted-foreground)] mt-1">webhooks awaiting delivery</p>
			</div>

			<!-- Failed Webhooks -->
			<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6">
				<p class="text-sm font-medium text-[var(--color-muted-foreground)]">Failed</p>
				<p class="text-2xl font-bold text-[var(--color-status-failed)] mt-2">{status.failedCount}</p>
				<p class="text-xs text-[var(--color-muted-foreground)] mt-1">permanent failures</p>
			</div>

			<!-- Dead Letter -->
			<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6">
				<p class="text-sm font-medium text-[var(--color-muted-foreground)]">Dead Letter</p>
				<p class="text-2xl font-bold text-[var(--color-status-dead-letter)] mt-2">{status.deadLetterCount}</p>
				<p class="text-xs text-[var(--color-muted-foreground)] mt-1">exceeded retry window</p>
			</div>
		</div>
	{/if}

	<!-- Quick Actions -->
	<div class="grid grid-cols-1 md:grid-cols-2 gap-4 mt-8">
		<a
			href="/endpoints/new"
			class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6 hover:border-[var(--color-foreground)] transition-colors"
		>
			<h3 class="text-lg font-semibold text-[var(--color-foreground)]">Create Endpoint</h3>
			<p class="text-sm text-[var(--color-muted-foreground)] mt-1">
				Set up a new webhook endpoint for Stripe, GitHub, or other providers
			</p>
		</a>
		<a
			href="/webhooks"
			class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6 hover:border-[var(--color-foreground)] transition-colors"
		>
			<h3 class="text-lg font-semibold text-[var(--color-foreground)]">View Webhooks</h3>
			<p class="text-sm text-[var(--color-muted-foreground)] mt-1">
				Browse and manage received webhooks, replay failed deliveries
			</p>
		</a>
	</div>
</div>
