<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { edgeClient, type Endpoint, ProviderType } from '$lib/api/client';

	let endpoint = $state<Endpoint | null>(null);
	let name = $state('');
	let signatureSecret = $state('');
	let destinationUrl = $state('');
	let loading = $state(true);
	let saving = $state(false);
	let error = $state<string | null>(null);

	$effect(() => {
		const id = $page.params.id;
		if (id) loadEndpoint(id);
	});

	async function loadEndpoint(id: string) {
		loading = true;
		error = null;
		try {
			const response = await edgeClient.getEndpoint({ id });
			endpoint = response.endpoint ?? null;
			if (endpoint) {
				name = endpoint.name;
				destinationUrl = endpoint.destinationUrl;
			}
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

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (!endpoint) return;

		saving = true;
		error = null;

		try {
			await edgeClient.updateEndpoint({
				id: endpoint.id,
				name: name !== endpoint.name ? name : undefined,
				destinationUrl: destinationUrl !== endpoint.destinationUrl ? destinationUrl : undefined,
				signatureSecret: signatureSecret || undefined
			});
			goto(`/endpoints/${endpoint.id}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to update endpoint';
		} finally {
			saving = false;
		}
	}
</script>

<div class="max-w-2xl space-y-6">
	{#if loading}
		<div class="animate-pulse space-y-4">
			<div class="h-8 w-48 bg-[var(--color-muted)] rounded"></div>
			<div class="h-4 w-96 bg-[var(--color-muted)] rounded"></div>
		</div>
	{:else if error && !endpoint}
		<div class="rounded-lg border border-[var(--color-destructive)] bg-[var(--color-destructive)]/10 p-4">
			<p class="text-[var(--color-destructive)]">{error}</p>
		</div>
	{:else if endpoint}
		<div>
			<a href="/endpoints/{endpoint.id}" class="text-sm text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]">
				← Back to Endpoint
			</a>
			<h1 class="text-2xl font-bold text-[var(--color-foreground)] mt-2">Edit Endpoint</h1>
			<p class="text-[var(--color-muted-foreground)]">{getProviderLabel(endpoint.providerType)} webhook endpoint</p>
		</div>

		{#if error}
			<div class="rounded-lg border border-[var(--color-destructive)] bg-[var(--color-destructive)]/10 p-4">
				<p class="text-[var(--color-destructive)]">{error}</p>
			</div>
		{/if}

		<form onsubmit={handleSubmit} class="space-y-4">
			<div class="space-y-2">
				<label for="name" class="text-sm font-medium text-[var(--color-foreground)]">Name</label>
				<input
					id="name"
					type="text"
					bind:value={name}
					required
					class="w-full px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)]"
				/>
			</div>

			<div class="space-y-2">
				<label for="provider" class="text-sm font-medium text-[var(--color-foreground)]">Provider</label>
				<input
					id="provider"
					type="text"
					value={getProviderLabel(endpoint.providerType)}
					disabled
					class="w-full px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-muted)] text-[var(--color-muted-foreground)] cursor-not-allowed"
				/>
				<p class="text-xs text-[var(--color-muted-foreground)]">
					Provider cannot be changed after creation
				</p>
			</div>

			<div class="space-y-2">
				<label for="signatureSecret" class="text-sm font-medium text-[var(--color-foreground)]">
					Signature Secret
					<span class="text-[var(--color-muted-foreground)] font-normal">(leave blank to keep existing)</span>
				</label>
				<input
					id="signatureSecret"
					type="password"
					bind:value={signatureSecret}
					placeholder="Enter new secret to change"
					class="w-full px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] placeholder:text-[var(--color-muted-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)] font-mono"
				/>
			</div>

			<div class="space-y-2">
				<label for="destinationUrl" class="text-sm font-medium text-[var(--color-foreground)]">Destination URL</label>
				<input
					id="destinationUrl"
					type="url"
					bind:value={destinationUrl}
					required
					class="w-full px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)] font-mono"
				/>
			</div>

			<div class="flex gap-4 pt-4">
				<button
					type="submit"
					disabled={saving}
					class="inline-flex items-center justify-center rounded-md bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-foreground)] hover:bg-[var(--color-primary)]/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
				>
					{saving ? 'Saving...' : 'Save Changes'}
				</button>
				<a
					href="/endpoints/{endpoint.id}"
					class="inline-flex items-center justify-center rounded-md border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-foreground)] hover:bg-[var(--color-muted)] transition-colors"
				>
					Cancel
				</a>
			</div>
		</form>
	{/if}
</div>
