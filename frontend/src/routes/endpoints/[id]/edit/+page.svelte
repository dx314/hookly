<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { edgeClient, type Endpoint, ProviderType } from '$lib/api/client';

	let endpoint = $state<Endpoint | null>(null);
	let name = $state('');
	let signatureSecret = $state('');
	let loading = $state(true);
	let saving = $state(false);
	let error = $state<string | null>(null);

	// Destination rows hold editable drafts next to the last saved values
	type DestinationRow = {
		id: string;
		name: string;
		url: string;
		enabled: boolean;
		savedName: string;
		savedUrl: string;
	};
	let rows = $state<DestinationRow[]>([]);
	let newName = $state('');
	let newUrl = $state('');
	let adding = $state(false);
	let busyId = $state<string | null>(null);
	let destError = $state<string | null>(null);

	let enabledCount = $derived(rows.filter((row) => row.enabled).length);

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
				syncRows(endpoint, true);
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

	function isDirty(row: DestinationRow): boolean {
		return row.name.trim() !== row.savedName || row.url.trim() !== row.savedUrl;
	}

	// Rebuild rows from the server state. Unsaved edits are kept, except for
	// the row that was just saved (savedId) or when discardDrafts is set.
	function syncRows(ep: Endpoint, discardDrafts = false, savedId?: string) {
		const drafts = new Map(rows.map((row) => [row.id, row]));
		rows = ep.destinations.map((dest) => {
			const draft = discardDrafts || dest.id === savedId ? undefined : drafts.get(dest.id);
			const keep = draft !== undefined && isDirty(draft);
			return {
				id: dest.id,
				name: keep ? draft.name : dest.name,
				url: keep ? draft.url : dest.url,
				enabled: dest.enabled,
				savedName: dest.name,
				savedUrl: dest.url
			};
		});
	}

	// Each destination RPC returns the updated endpoint; fall back to a refetch
	async function applyEndpoint(updated: Endpoint | undefined, savedId?: string) {
		if (!updated && endpoint) {
			updated = (await edgeClient.getEndpoint({ id: endpoint.id })).endpoint;
		}
		if (updated) {
			endpoint = updated;
			syncRows(updated, false, savedId);
		}
	}

	async function saveDestination(e: Event, row: DestinationRow) {
		e.preventDefault();
		if (!isDirty(row)) return;
		busyId = row.id;
		destError = null;
		try {
			const rowName = row.name.trim();
			const rowUrl = row.url.trim();
			const response = await edgeClient.updateDestination({
				id: row.id,
				name: rowName !== row.savedName ? rowName : undefined,
				url: rowUrl !== row.savedUrl ? rowUrl : undefined
			});
			await applyEndpoint(response.endpoint, row.id);
		} catch (e) {
			destError = e instanceof Error ? e.message : 'Failed to update destination';
		} finally {
			busyId = null;
		}
	}

	function resetDestination(row: DestinationRow) {
		row.name = row.savedName;
		row.url = row.savedUrl;
	}

	async function toggleDestination(row: DestinationRow) {
		busyId = row.id;
		destError = null;
		try {
			const response = await edgeClient.updateDestination({ id: row.id, enabled: !row.enabled });
			await applyEndpoint(response.endpoint);
		} catch (e) {
			destError = e instanceof Error ? e.message : 'Failed to update destination';
		} finally {
			busyId = null;
		}
	}

	async function removeDestination(row: DestinationRow) {
		if (!confirm(`Remove destination "${row.savedName}"? Its pending deliveries will be abandoned and its delivery history will be deleted. This cannot be undone.`)) {
			return;
		}
		busyId = row.id;
		destError = null;
		try {
			const response = await edgeClient.removeDestination({ id: row.id });
			await applyEndpoint(response.endpoint);
		} catch (e) {
			destError = e instanceof Error ? e.message : 'Failed to remove destination';
		} finally {
			busyId = null;
		}
	}

	async function addDestination(e: Event) {
		e.preventDefault();
		if (!endpoint) return;
		adding = true;
		destError = null;
		try {
			const response = await edgeClient.addDestination({
				endpointId: endpoint.id,
				name: newName.trim(),
				url: newUrl.trim()
			});
			newName = '';
			newUrl = '';
			await applyEndpoint(response.endpoint);
		} catch (e) {
			destError = e instanceof Error ? e.message : 'Failed to add destination';
		} finally {
			adding = false;
		}
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (!endpoint) return;

		if (rows.some(isDirty)) {
			error = 'You have unsaved destination changes. Save or reset them before leaving.';
			return;
		}

		saving = true;
		error = null;

		try {
			await edgeClient.updateEndpoint({
				id: endpoint.id,
				name: name !== endpoint.name ? name : undefined,
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

		<!-- Destinations -->
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6 space-y-4">
			<div>
				<h2 class="text-lg font-semibold text-[var(--color-foreground)]">Destinations</h2>
				<p class="text-sm text-[var(--color-muted-foreground)]">
					Every webhook is delivered to each enabled destination independently. The first destination is the
					primary. Changes here are applied immediately.
				</p>
			</div>

			{#if destError}
				<div class="rounded-lg border border-[var(--color-destructive)] bg-[var(--color-destructive)]/10 p-4">
					<p class="text-[var(--color-destructive)]">{destError}</p>
				</div>
			{/if}

			<div class="space-y-3">
				{#each rows as row, i (row.id)}
					{@const busy = busyId === row.id}
					<form
						onsubmit={(e) => saveDestination(e, row)}
						class="rounded-md border border-[var(--color-border)] p-3 space-y-3"
					>
						<div class="flex items-start gap-2">
							<input
								type="text"
								bind:value={row.name}
								required
								placeholder="name"
								aria-label="Destination name"
								class="w-1/4 min-w-0 px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] placeholder:text-[var(--color-muted-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)]"
							/>
							<input
								type="url"
								bind:value={row.url}
								required
								placeholder="http://localhost:3000/webhooks"
								aria-label="Destination URL"
								class="flex-1 min-w-0 px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] placeholder:text-[var(--color-muted-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)] font-mono"
							/>
						</div>
						<div class="flex flex-wrap items-center justify-between gap-2">
							<div class="flex items-center gap-2">
								{#if row.enabled}
									<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400">
										Enabled
									</span>
								{:else}
									<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium bg-[var(--color-muted)] text-[var(--color-muted-foreground)]">
										Disabled
									</span>
								{/if}
								{#if i === 0}
									<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium border border-[var(--color-border)] text-[var(--color-muted-foreground)]">
										Primary
									</span>
								{/if}
								{#if isDirty(row)}
									<span class="text-xs text-[var(--color-muted-foreground)]">Unsaved changes</span>
								{/if}
							</div>
							<div class="flex items-center gap-2">
								{#if isDirty(row)}
									<button
										type="submit"
										disabled={busy}
										class="text-xs px-2 py-1 rounded bg-[var(--color-primary)] text-[var(--color-primary-foreground)] hover:bg-[var(--color-primary)]/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
									>
										{busy ? 'Saving...' : 'Save'}
									</button>
									<button
										type="button"
										onclick={() => resetDestination(row)}
										disabled={busy}
										class="text-xs px-2 py-1 rounded border border-[var(--color-border)] text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)] hover:border-[var(--color-foreground)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
									>
										Reset
									</button>
								{/if}
								<button
									type="button"
									onclick={() => toggleDestination(row)}
									disabled={busy || (row.enabled && enabledCount <= 1)}
									title={row.enabled && enabledCount <= 1 ? 'At least one destination must stay enabled' : undefined}
									class="text-xs px-2 py-1 rounded border border-[var(--color-border)] text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)] hover:border-[var(--color-foreground)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
								>
									{row.enabled ? 'Disable' : 'Enable'}
								</button>
								<button
									type="button"
									onclick={() => removeDestination(row)}
									disabled={busy || rows.length <= 1}
									title={rows.length <= 1 ? 'An endpoint needs at least one destination' : undefined}
									class="text-xs px-2 py-1 rounded border border-[var(--color-destructive)] text-[var(--color-destructive)] hover:bg-[var(--color-destructive)] hover:text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
								>
									Remove
								</button>
							</div>
						</div>
					</form>
				{/each}
			</div>

			<p class="text-xs text-[var(--color-muted-foreground)]">
				Removing a destination abandons its pending deliveries and deletes its delivery history. Disable it
				instead to pause forwarding while keeping the history.
			</p>

			<form onsubmit={addDestination} class="space-y-2 pt-4 border-t border-[var(--color-border)]">
				<span class="text-sm font-medium text-[var(--color-foreground)]">Add destination</span>
				<div class="flex items-start gap-2">
					<input
						type="text"
						bind:value={newName}
						required
						placeholder="name"
						aria-label="New destination name"
						class="w-1/4 min-w-0 px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] placeholder:text-[var(--color-muted-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)]"
					/>
					<input
						type="url"
						bind:value={newUrl}
						required
						placeholder="http://localhost:4000/webhooks"
						aria-label="New destination URL"
						class="flex-1 min-w-0 px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] placeholder:text-[var(--color-muted-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)] font-mono"
					/>
					<button
						type="submit"
						disabled={adding}
						class="px-3 py-2 rounded-md border border-[var(--color-border)] text-sm font-medium text-[var(--color-foreground)] hover:bg-[var(--color-muted)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
					>
						{adding ? 'Adding...' : 'Add'}
					</button>
				</div>
				<p class="text-xs text-[var(--color-muted-foreground)]">
					A new destination only receives webhooks that arrive after it is added. Earlier webhooks are not
					sent to it, and replaying them will not include it.
				</p>
			</form>
		</div>
	{/if}
</div>
