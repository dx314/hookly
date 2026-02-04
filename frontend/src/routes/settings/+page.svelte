<script lang="ts">
	import { onMount } from 'svelte';
	import { edgeClient } from '$lib/api/client';

	let settings = $state<{
		baseUrl: string;
		githubAuthEnabled: boolean;
		telegramNotificationsEnabled: boolean;
	} | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	onMount(async () => {
		try {
			const response = await edgeClient.getSettings({});
			settings = {
				baseUrl: response.baseUrl,
				githubAuthEnabled: response.githubAuthEnabled,
				telegramNotificationsEnabled: response.telegramNotificationsEnabled
			};
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to fetch settings';
		} finally {
			loading = false;
		}
	});
</script>

<div class="space-y-6">
	<div>
		<h1 class="text-2xl font-bold text-[var(--color-foreground)]">Settings</h1>
		<p class="text-[var(--color-muted-foreground)]">System configuration (read-only)</p>
	</div>

	{#if loading}
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)]">
			<div class="p-8 text-center text-[var(--color-muted-foreground)]">Loading...</div>
		</div>
	{:else if error}
		<div class="rounded-lg border border-[var(--color-destructive)] bg-[var(--color-destructive)]/10 p-4">
			<p class="text-[var(--color-destructive)]">{error}</p>
		</div>
	{:else if settings}
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] divide-y divide-[var(--color-border)]">
			<div class="p-6">
				<h2 class="text-lg font-semibold text-[var(--color-foreground)] mb-4">General</h2>
				<dl class="space-y-4 text-sm">
					<div class="flex justify-between">
						<dt class="text-[var(--color-muted-foreground)]">Base URL</dt>
						<dd class="font-mono">{settings.baseUrl}</dd>
					</div>
				</dl>
			</div>

			<div class="p-6">
				<h2 class="text-lg font-semibold text-[var(--color-foreground)] mb-4">Authentication</h2>
				<dl class="space-y-4 text-sm">
					<div class="flex justify-between items-center">
						<dt class="text-[var(--color-muted-foreground)]">GitHub OAuth</dt>
						<dd>
							{#if settings.githubAuthEnabled}
								<span class="inline-flex items-center rounded-full px-2 py-1 text-xs font-medium bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400">
									Enabled
								</span>
							{:else}
								<span class="inline-flex items-center rounded-full px-2 py-1 text-xs font-medium bg-[var(--color-muted)] text-[var(--color-muted-foreground)]">
									Disabled
								</span>
							{/if}
						</dd>
					</div>
				</dl>
			</div>

			<div class="p-6">
				<h2 class="text-lg font-semibold text-[var(--color-foreground)] mb-4">Notifications</h2>
				<dl class="space-y-4 text-sm">
					<div class="flex justify-between items-center">
						<dt class="text-[var(--color-muted-foreground)]">Telegram Notifications</dt>
						<dd>
							{#if settings.telegramNotificationsEnabled}
								<span class="inline-flex items-center rounded-full px-2 py-1 text-xs font-medium bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400">
									Enabled
								</span>
							{:else}
								<span class="inline-flex items-center rounded-full px-2 py-1 text-xs font-medium bg-[var(--color-muted)] text-[var(--color-muted-foreground)]">
									Disabled
								</span>
							{/if}
						</dd>
					</div>
				</dl>
			</div>
		</div>

		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-muted)]/50 p-4">
			<p class="text-sm text-[var(--color-muted-foreground)]">
				Settings are configured via environment variables on the server. See the documentation for available options.
			</p>
		</div>
	{/if}
</div>
