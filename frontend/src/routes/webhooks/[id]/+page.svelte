<script lang="ts">
	import { page } from '$app/stores';
	import { edgeClient, type Webhook, WebhookStatus } from '$lib/api/client';

	let webhook = $state<Webhook | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	// 'all' while replaying every destination, otherwise the destination id being replayed
	let replaying = $state<string | null>(null);
	let replayError = $state<string | null>(null);
	let showHeaders = $state(false);
	let showPayload = $state(true);

	$effect(() => {
		const id = $page.params.id;
		if (id) loadWebhook(id);
	});

	async function loadWebhook(id: string) {
		loading = true;
		error = null;
		try {
			const response = await edgeClient.getWebhook({ id });
			webhook = response.webhook ?? null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to fetch webhook';
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

	function formatPayload(payload: Uint8Array): string {
		try {
			const text = new TextDecoder().decode(payload);
			const json = JSON.parse(text);
			return JSON.stringify(json, null, 2);
		} catch {
			return new TextDecoder().decode(payload);
		}
	}

	function formatHeaders(headers: Record<string, string>): string {
		return JSON.stringify(headers, null, 2);
	}

	// Replays to a single destination, or to all of them when destinationId is omitted
	async function replayWebhook(destinationId?: string) {
		if (!webhook) return;
		replaying = destinationId ?? 'all';
		replayError = null;
		try {
			const response = await edgeClient.replayWebhook({ id: webhook.id, destinationId });
			if (response.webhook) {
				webhook = response.webhook;
			}
		} catch (e) {
			replayError = e instanceof Error ? e.message : 'Failed to replay webhook';
		} finally {
			replaying = null;
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
	{:else if webhook}
		{@const status = getStatusBadge(webhook.status)}
		<div>
			<a href="/webhooks" class="text-sm text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]">
				← Back to Webhooks
			</a>
			<div class="flex items-center gap-4 mt-2">
				<h1 class="text-2xl font-bold text-[var(--color-foreground)]">Webhook Details</h1>
				<span class="{status.class} inline-flex items-center rounded-full px-2 py-1 text-xs font-medium">
					{status.label}
				</span>
			</div>
			<p class="text-[var(--color-muted-foreground)] font-mono text-sm">{webhook.id}</p>
		</div>

		<!-- Actions -->
		{#if webhook.status !== WebhookStatus.PENDING}
			<div class="flex gap-2">
				<button
					onclick={() => replayWebhook()}
					disabled={replaying !== null}
					class="inline-flex items-center justify-center rounded-md bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-foreground)] hover:bg-[var(--color-primary)]/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
				>
					{#if replaying === 'all'}
						Replaying...
					{:else}
						{webhook.deliveries.length > 1 ? 'Replay all' : 'Replay Webhook'}
					{/if}
				</button>
			</div>
		{/if}

		{#if replayError}
			<div class="rounded-lg border border-[var(--color-destructive)] bg-[var(--color-destructive)]/10 p-4">
				<p class="text-[var(--color-destructive)]">{replayError}</p>
			</div>
		{/if}

		<!-- Details -->
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6">
			<h2 class="text-lg font-semibold text-[var(--color-foreground)] mb-4">Information</h2>
			<dl class="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm">
				<div>
					<dt class="text-[var(--color-muted-foreground)]">Endpoint</dt>
					<dd class="mt-1">
						<a href="/endpoints/{webhook.endpointId}" class="hover:underline">{webhook.endpointId}</a>
					</dd>
				</div>
				<div>
					<dt class="text-[var(--color-muted-foreground)]">Received</dt>
					<dd class="mt-1">{formatDate(webhook.receivedAt)}</dd>
				</div>
				<div>
					<dt class="text-[var(--color-muted-foreground)]">Signature Valid</dt>
					<dd class="mt-1">{webhook.signatureValid ? '✓ Yes' : '✗ No'}</dd>
				</div>
				<div>
					<dt class="text-[var(--color-muted-foreground)]">Delivery Attempts</dt>
					<dd class="mt-1">{webhook.attempts}</dd>
				</div>
				{#if webhook.lastAttemptAt}
					<div>
						<dt class="text-[var(--color-muted-foreground)]">Last Attempt</dt>
						<dd class="mt-1">{formatDate(webhook.lastAttemptAt)}</dd>
					</div>
				{/if}
				{#if webhook.deliveredAt}
					<div>
						<dt class="text-[var(--color-muted-foreground)]">Delivered</dt>
						<dd class="mt-1">{formatDate(webhook.deliveredAt)}</dd>
					</div>
				{/if}
				{#if webhook.errorMessage}
					<div class="md:col-span-2">
						<dt class="text-[var(--color-muted-foreground)]">Error Message</dt>
						<dd class="mt-1 text-[var(--color-destructive)]">{webhook.errorMessage}</dd>
					</div>
				{/if}
			</dl>
		</div>

		<!-- Deliveries -->
		{#if webhook.deliveries.length > 0}
			<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] overflow-hidden">
				<div class="px-6 py-4 border-b border-[var(--color-border)]">
					<h2 class="text-lg font-semibold text-[var(--color-foreground)]">Deliveries</h2>
					<p class="text-sm text-[var(--color-muted-foreground)]">
						One delivery per destination; the status above is a rollup of these
					</p>
				</div>
				<div class="overflow-x-auto">
					<table class="w-full">
						<thead class="bg-[var(--color-muted)]">
							<tr>
								<th class="text-left px-4 py-2 text-xs font-medium text-[var(--color-muted-foreground)]">Destination</th>
								<th class="text-left px-4 py-2 text-xs font-medium text-[var(--color-muted-foreground)]">Status</th>
								<th class="text-right px-4 py-2 text-xs font-medium text-[var(--color-muted-foreground)]">Attempts</th>
								<th class="text-left px-4 py-2 text-xs font-medium text-[var(--color-muted-foreground)]">Last Attempt</th>
								<th class="text-left px-4 py-2 text-xs font-medium text-[var(--color-muted-foreground)]">Delivered</th>
								<th class="text-right px-4 py-2 text-xs font-medium text-[var(--color-muted-foreground)]">Actions</th>
							</tr>
						</thead>
						<tbody class="divide-y divide-[var(--color-border)]">
							{#each webhook.deliveries as delivery (delivery.id)}
								{@const deliveryStatus = getStatusBadge(delivery.status)}
								<tr class="hover:bg-[var(--color-muted)]/50 align-top">
									<td class="px-4 py-2">
										<div class="text-sm font-medium text-[var(--color-foreground)]">{delivery.destinationName}</div>
										<div class="font-mono text-xs text-[var(--color-muted-foreground)] mt-1 break-all">{delivery.destinationUrl}</div>
										{#if delivery.errorMessage}
											<div class="text-xs text-[var(--color-destructive)] mt-1 break-words">{delivery.errorMessage}</div>
										{/if}
									</td>
									<td class="px-4 py-2">
										<span class="{deliveryStatus.class} inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium whitespace-nowrap">
											{deliveryStatus.label}
										</span>
									</td>
									<td class="px-4 py-2 text-sm text-right tabular-nums text-[var(--color-muted-foreground)]">{delivery.attempts}</td>
									<td class="px-4 py-2 text-sm text-[var(--color-muted-foreground)] whitespace-nowrap">
										{delivery.lastAttemptAt ? formatDate(delivery.lastAttemptAt) : '—'}
									</td>
									<td class="px-4 py-2 text-sm text-[var(--color-muted-foreground)] whitespace-nowrap">
										{delivery.deliveredAt ? formatDate(delivery.deliveredAt) : '—'}
									</td>
									<td class="px-4 py-2 text-right">
										{#if delivery.status !== WebhookStatus.PENDING}
											<button
												onclick={() => replayWebhook(delivery.destinationId)}
												disabled={replaying !== null}
												class="text-xs px-2 py-1 rounded border border-[var(--color-border)] text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)] hover:border-[var(--color-foreground)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
											>
												{replaying === delivery.destinationId ? 'Replaying...' : 'Replay'}
											</button>
										{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		{/if}

		<!-- Headers -->
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] overflow-hidden">
			<button
				onclick={() => showHeaders = !showHeaders}
				class="w-full px-6 py-4 flex items-center justify-between hover:bg-[var(--color-muted)]/50 transition-colors"
			>
				<h2 class="text-lg font-semibold text-[var(--color-foreground)]">Headers</h2>
				<span class="text-[var(--color-muted-foreground)]">{showHeaders ? '−' : '+'}</span>
			</button>
			{#if showHeaders && webhook.headers}
				<div class="px-6 pb-6">
					<pre class="bg-[var(--color-muted)] p-4 rounded-md overflow-x-auto text-sm font-mono">{formatHeaders(webhook.headers)}</pre>
				</div>
			{/if}
		</div>

		<!-- Payload -->
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] overflow-hidden">
			<button
				onclick={() => showPayload = !showPayload}
				class="w-full px-6 py-4 flex items-center justify-between hover:bg-[var(--color-muted)]/50 transition-colors"
			>
				<h2 class="text-lg font-semibold text-[var(--color-foreground)]">Payload</h2>
				<span class="text-[var(--color-muted-foreground)]">{showPayload ? '−' : '+'}</span>
			</button>
			{#if showPayload && webhook.payload}
				<div class="px-6 pb-6">
					<pre class="bg-[var(--color-muted)] p-4 rounded-md overflow-x-auto text-sm font-mono max-h-[500px] overflow-y-auto">{formatPayload(webhook.payload)}</pre>
				</div>
			{/if}
		</div>
	{/if}
</div>
