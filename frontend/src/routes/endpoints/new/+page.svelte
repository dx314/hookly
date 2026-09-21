<script lang="ts">
	import { goto } from '$app/navigation';
	import { edgeClient, ProviderType } from '$lib/api/client';

	let name = $state('');
	let providerType = $state<ProviderType>(ProviderType.GENERIC);
	let signatureSecret = $state('');
	// key is a client-only stable identity for keyed rendering; it is never sent to the API
	let nextKey = 1;
	let destinations = $state<{ key: number; name: string; url: string }[]>([
		{ key: 0, name: 'default', url: '' }
	]);
	let loading = $state(false);
	let error = $state<string | null>(null);

	const providerOptions = [
		{ value: ProviderType.STRIPE, label: 'Stripe' },
		{ value: ProviderType.GITHUB, label: 'GitHub' },
		{ value: ProviderType.TELEGRAM, label: 'Telegram' },
		{ value: ProviderType.GENERIC, label: 'Generic / Other' }
	];

	function addDestination() {
		destinations.push({ key: nextKey++, name: '', url: '' });
	}

	function removeDestination(index: number) {
		if (destinations.length <= 1) return;
		destinations.splice(index, 1);
	}

	function validateDestinations(): string | null {
		const names = destinations.map((dest) => dest.name.trim());
		for (const [i, dest] of destinations.entries()) {
			const destName = names[i];
			if (!destName) return 'Every destination needs a name';
			if (!dest.url.trim()) return 'Every destination needs a URL';
			if (names.indexOf(destName) !== i) return `Duplicate destination name "${destName}"`;
		}
		return null;
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = validateDestinations();
		if (error) return;

		loading = true;

		try {
			const response = await edgeClient.createEndpoint({
				name,
				providerType,
				signatureSecret,
				destinations: destinations.map((dest) => ({
					name: dest.name.trim(),
					url: dest.url.trim()
				}))
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
				{#each providerOptions as option (option.value)}
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
			<div class="flex items-center justify-between">
				<span class="text-sm font-medium text-[var(--color-foreground)]">Destinations</span>
				<button
					type="button"
					onclick={addDestination}
					class="text-xs px-2 py-1 rounded border border-[var(--color-border)] text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)] hover:border-[var(--color-foreground)] transition-colors"
				>
					+ Add destination
				</button>
			</div>
			<div class="space-y-2">
				{#each destinations as dest, i (dest.key)}
					<div class="flex items-start gap-2">
						<input
							type="text"
							bind:value={dest.name}
							required
							placeholder="name"
							aria-label="Destination {i + 1} name"
							class="w-1/4 min-w-0 px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] placeholder:text-[var(--color-muted-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)]"
						/>
						<input
							type="url"
							bind:value={dest.url}
							required
							placeholder="http://localhost:3000/webhooks/stripe"
							aria-label="Destination {i + 1} URL"
							class="flex-1 min-w-0 px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] placeholder:text-[var(--color-muted-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)] font-mono"
						/>
						<button
							type="button"
							onclick={() => removeDestination(i)}
							disabled={destinations.length <= 1}
							class="px-3 py-2 rounded-md border border-[var(--color-border)] text-sm text-[var(--color-muted-foreground)] hover:text-[var(--color-destructive)] hover:border-[var(--color-destructive)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
						>
							Remove
						</button>
					</div>
				{/each}
			</div>
			<p class="text-xs text-[var(--color-muted-foreground)]">
				URLs on your private network where webhooks will be forwarded. Every webhook is delivered to
				each destination independently; the first one is the primary. Names must be unique.
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
