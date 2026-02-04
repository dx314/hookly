<script lang="ts">
	import { goto } from '$app/navigation';
	import { edgeClient, ProviderType } from '$lib/api/client';

	let name = $state('');
	let providerType = $state<ProviderType>(ProviderType.GENERIC);
	let signatureSecret = $state('');
	let destinationUrl = $state('');
	let loading = $state(false);
	let error = $state<string | null>(null);

	const providerOptions = [
		{ value: ProviderType.STRIPE, label: 'Stripe' },
		{ value: ProviderType.GITHUB, label: 'GitHub' },
		{ value: ProviderType.TELEGRAM, label: 'Telegram' },
		{ value: ProviderType.GENERIC, label: 'Generic / Other' }
	];

	async function handleSubmit(e: Event) {
		e.preventDefault();
		loading = true;
		error = null;

		try {
			const response = await edgeClient.createEndpoint({
				name,
				providerType,
				signatureSecret,
				destinationUrl
			});
			goto(`/endpoints/${response.endpoint?.id}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to create endpoint';
		} finally {
			loading = false;
		}
	}
</script>

<div class="max-w-2xl space-y-6">
	<div>
		<a href="/endpoints" class="text-sm text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]">
			← Back to Endpoints
		</a>
		<h1 class="text-2xl font-bold text-[var(--color-foreground)] mt-2">Create Endpoint</h1>
		<p class="text-[var(--color-muted-foreground)]">Set up a new webhook endpoint</p>
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
				placeholder="My Stripe Webhooks"
				class="w-full px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] placeholder:text-[var(--color-muted-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)]"
			/>
		</div>

		<div class="space-y-2">
			<label for="provider" class="text-sm font-medium text-[var(--color-foreground)]">Provider</label>
			<select
				id="provider"
				bind:value={providerType}
				class="w-full px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)]"
			>
				{#each providerOptions as option}
					<option value={option.value}>{option.label}</option>
				{/each}
			</select>
			<p class="text-xs text-[var(--color-muted-foreground)]">
				Provider determines how webhook signatures are verified
			</p>
		</div>

		<div class="space-y-2">
			<label for="signatureSecret" class="text-sm font-medium text-[var(--color-foreground)]">
				Signature Secret
				<span class="text-[var(--color-muted-foreground)] font-normal">(optional)</span>
			</label>
			<input
				id="signatureSecret"
				type="password"
				bind:value={signatureSecret}
				placeholder="whsec_..."
				class="w-full px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] placeholder:text-[var(--color-muted-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)] font-mono"
			/>
			<p class="text-xs text-[var(--color-muted-foreground)]">
				Secret used to verify webhook signatures. Get this from your provider's dashboard.
			</p>
		</div>

		<div class="space-y-2">
			<label for="destinationUrl" class="text-sm font-medium text-[var(--color-foreground)]">Destination URL</label>
			<input
				id="destinationUrl"
				type="url"
				bind:value={destinationUrl}
				required
				placeholder="http://localhost:3000/webhooks/stripe"
				class="w-full px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] placeholder:text-[var(--color-muted-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)] font-mono"
			/>
			<p class="text-xs text-[var(--color-muted-foreground)]">
				The URL on your private network where webhooks will be forwarded
			</p>
		</div>

		<div class="flex gap-4 pt-4">
			<button
				type="submit"
				disabled={loading}
				class="inline-flex items-center justify-center rounded-md bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-foreground)] hover:bg-[var(--color-primary)]/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
			>
				{loading ? 'Creating...' : 'Create Endpoint'}
			</button>
			<a
				href="/endpoints"
				class="inline-flex items-center justify-center rounded-md border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-foreground)] hover:bg-[var(--color-muted)] transition-colors"
			>
				Cancel
			</a>
		</div>
	</form>
</div>
