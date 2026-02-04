<script lang="ts">
	import { onMount } from 'svelte';
	import { edgeClient, type UserSettings, type SystemSettings } from '$lib/api/client';
	import { theme, stringToThemePreference, type Theme } from '$lib/stores/theme';

	let userSettings = $state<UserSettings | null>(null);
	let systemSettings = $state<SystemSettings | null>(null);
	let isSuperuser = $state(false);
	let loading = $state(true);
	let error = $state<string | null>(null);

	// Form state
	let telegramBotToken = $state('');
	let telegramChatId = $state('');
	let telegramEnabled = $state(false);
	let savingTelegram = $state(false);
	let telegramSaveMessage = $state<{ type: 'success' | 'error'; text: string } | null>(null);

	const themes: { value: Theme; label: string; description: string }[] = [
		{ value: 'system', label: 'System', description: 'Follow your device settings' },
		{ value: 'light', label: 'Light', description: 'Classic light theme' },
		{ value: 'dark', label: 'Dark', description: 'Easy on the eyes' },
		{ value: 'placid-blue-light', label: 'Placid Blue Light', description: 'Calm blue tones' },
		{ value: 'placid-blue-dark', label: 'Placid Blue Dark', description: 'Deep blue night mode' }
	];

	onMount(async () => {
		try {
			// Get user settings
			const userResponse = await edgeClient.getUserSettings({});
			userSettings = userResponse.settings ?? null;

			if (userSettings) {
				telegramChatId = userSettings.telegramChatId ?? '';
				telegramEnabled = userSettings.telegramEnabled;
				isSuperuser = userSettings.isSuperuser;
			}

			// Get system settings if superuser
			if (isSuperuser) {
				try {
					const systemResponse = await edgeClient.getSystemSettings({});
					systemSettings = systemResponse.settings ?? null;
				} catch {
					// Not authorized or error - ignore
				}
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to fetch settings';
		} finally {
			loading = false;
		}
	});

	async function saveTelegramSettings() {
		if (savingTelegram) return;
		savingTelegram = true;
		telegramSaveMessage = null;

		try {
			const response = await edgeClient.updateUserSettings({
				telegramBotToken: telegramBotToken || undefined,
				telegramChatId: telegramChatId || undefined,
				telegramEnabled: telegramEnabled
			});
			userSettings = response.settings ?? null;
			telegramBotToken = ''; // Clear the token field after save
			telegramSaveMessage = { type: 'success', text: 'Telegram settings saved!' };
		} catch (e) {
			telegramSaveMessage = {
				type: 'error',
				text: e instanceof Error ? e.message : 'Failed to save settings'
			};
		} finally {
			savingTelegram = false;
		}
	}

	async function selectTheme(newTheme: Theme) {
		const oldTheme = $theme;
		theme.set(newTheme);

		try {
			await edgeClient.updateUserSettings({
				themePreference: stringToThemePreference(newTheme)
			});
		} catch {
			// Revert on error
			theme.set(oldTheme);
		}
	}

	function formatDate(timestamp: { seconds: bigint } | undefined): string {
		if (!timestamp) return 'Never';
		const date = new Date(Number(timestamp.seconds) * 1000);
		return date.toLocaleString();
	}
</script>

<div class="space-y-8">
	<div>
		<h1 class="text-2xl font-bold text-[var(--color-foreground)]">Settings</h1>
		<p class="text-[var(--color-muted-foreground)]">Manage your account and preferences</p>
	</div>

	{#if loading}
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)]">
			<div class="p-8 text-center text-[var(--color-muted-foreground)]">Loading...</div>
		</div>
	{:else if error}
		<div class="rounded-lg border border-[var(--color-destructive)] bg-[var(--color-destructive)]/10 p-4">
			<p class="text-[var(--color-destructive)]">{error}</p>
		</div>
	{:else if userSettings}
		<!-- Profile Section -->
		<section class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)]">
			<div class="p-6 border-b border-[var(--color-border)]">
				<h2 class="text-lg font-semibold text-[var(--color-foreground)]">Profile</h2>
				<p class="text-sm text-[var(--color-muted-foreground)]">Your GitHub account information</p>
			</div>
			<div class="p-6">
				<div class="flex items-start gap-6">
					{#if userSettings.avatarUrl}
						<img
							src={userSettings.avatarUrl}
							alt={userSettings.username}
							class="w-20 h-20 rounded-full"
						/>
					{:else}
						<div class="w-20 h-20 rounded-full bg-[var(--color-muted)] flex items-center justify-center text-2xl">
							{userSettings.username.charAt(0).toUpperCase()}
						</div>
					{/if}
					<div class="flex-1 space-y-3">
						<div>
							<p class="text-lg font-medium text-[var(--color-foreground)]">
								{userSettings.githubName || userSettings.username}
							</p>
							<p class="text-sm text-[var(--color-muted-foreground)]">@{userSettings.username}</p>
						</div>
						{#if userSettings.githubEmail}
							<p class="text-sm text-[var(--color-muted-foreground)]">
								{userSettings.githubEmail}
							</p>
						{/if}
						{#if userSettings.githubProfileUrl}
							<a
								href={userSettings.githubProfileUrl}
								target="_blank"
								rel="noopener noreferrer"
								class="text-sm text-[var(--color-primary)] hover:underline"
							>
								View GitHub Profile
							</a>
						{/if}
						<p class="text-xs text-[var(--color-muted-foreground)]">
							Last login: {formatDate(userSettings.lastLoginAt)}
						</p>
					</div>
				</div>
			</div>
		</section>

		<!-- Notifications Section -->
		<section class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)]">
			<div class="p-6 border-b border-[var(--color-border)]">
				<h2 class="text-lg font-semibold text-[var(--color-foreground)]">Notifications</h2>
				<p class="text-sm text-[var(--color-muted-foreground)]">
					Configure Telegram notifications for webhook failures
				</p>
			</div>
			<div class="p-6 space-y-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="font-medium text-[var(--color-foreground)]">Enable Telegram Notifications</p>
						<p class="text-sm text-[var(--color-muted-foreground)]">
							Receive alerts when webhooks fail to deliver
						</p>
					</div>
					<label class="relative inline-flex items-center cursor-pointer">
						<input
							type="checkbox"
							bind:checked={telegramEnabled}
							class="sr-only peer"
						/>
						<div class="w-11 h-6 bg-[var(--color-muted)] peer-focus:ring-2 peer-focus:ring-[var(--color-ring)] rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-[var(--color-primary)]"></div>
					</label>
				</div>

				<div class="space-y-4">
					<div>
						<label for="telegram-token" class="block text-sm font-medium text-[var(--color-foreground)] mb-1">
							Bot Token
							{#if userSettings.telegramConfigured}
								<span class="text-xs text-[var(--color-muted-foreground)]">(configured)</span>
							{/if}
						</label>
						<input
							id="telegram-token"
							type="password"
							bind:value={telegramBotToken}
							placeholder={userSettings.telegramConfigured ? '••••••••••••' : 'Enter your Telegram bot token'}
							class="w-full px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] placeholder:text-[var(--color-muted-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)]"
						/>
						<p class="mt-1 text-xs text-[var(--color-muted-foreground)]">
							Create a bot via <a href="https://t.me/BotFather" target="_blank" class="text-[var(--color-primary)] hover:underline">@BotFather</a>
						</p>
					</div>

					<div>
						<label for="telegram-chat" class="block text-sm font-medium text-[var(--color-foreground)] mb-1">
							Chat ID
						</label>
						<input
							id="telegram-chat"
							type="text"
							bind:value={telegramChatId}
							placeholder="Enter your chat ID"
							class="w-full px-3 py-2 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] text-[var(--color-foreground)] placeholder:text-[var(--color-muted-foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--color-ring)]"
						/>
						<p class="mt-1 text-xs text-[var(--color-muted-foreground)]">
							Send /start to your bot, then use <a href="https://t.me/userinfobot" target="_blank" class="text-[var(--color-primary)] hover:underline">@userinfobot</a> to find your ID
						</p>
					</div>
				</div>

				{#if telegramSaveMessage}
					<div
						class="p-3 rounded-md text-sm {telegramSaveMessage.type === 'success'
							? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
							: 'bg-[var(--color-destructive)]/10 text-[var(--color-destructive)]'}"
					>
						{telegramSaveMessage.text}
					</div>
				{/if}

				<button
					onclick={saveTelegramSettings}
					disabled={savingTelegram}
					class="px-4 py-2 rounded-md bg-[var(--color-primary)] text-[var(--color-primary-foreground)] font-medium hover:opacity-90 disabled:opacity-50 transition-opacity"
				>
					{savingTelegram ? 'Saving...' : 'Save Notification Settings'}
				</button>
			</div>
		</section>

		<!-- Appearance Section -->
		<section class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)]">
			<div class="p-6 border-b border-[var(--color-border)]">
				<h2 class="text-lg font-semibold text-[var(--color-foreground)]">Appearance</h2>
				<p class="text-sm text-[var(--color-muted-foreground)]">Choose your preferred theme</p>
			</div>
			<div class="p-6">
				<div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-3">
					{#each themes as t (t.value)}
						<button
							onclick={() => selectTheme(t.value)}
							class="p-4 rounded-lg border-2 text-left transition-all {$theme === t.value
								? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10'
								: 'border-[var(--color-border)] hover:border-[var(--color-muted-foreground)]'}"
						>
							<p class="font-medium text-sm text-[var(--color-foreground)]">{t.label}</p>
							<p class="text-xs text-[var(--color-muted-foreground)] mt-1">{t.description}</p>
						</button>
					{/each}
				</div>
			</div>
		</section>

		<!-- System Settings Section (Superuser Only) -->
		{#if isSuperuser && systemSettings}
			<section class="rounded-lg border border-amber-500/50 bg-amber-500/5">
				<div class="p-6 border-b border-amber-500/30">
					<div class="flex items-center gap-2">
						<span class="text-lg">👑</span>
						<h2 class="text-lg font-semibold text-[var(--color-foreground)]">System Settings</h2>
					</div>
					<p class="text-sm text-[var(--color-muted-foreground)]">Administrator-only configuration</p>
				</div>
				<div class="p-6 space-y-4">
					<div class="grid grid-cols-2 gap-6">
						<div>
							<p class="text-sm text-[var(--color-muted-foreground)]">Base URL</p>
							<p class="font-mono text-sm text-[var(--color-foreground)]">{systemSettings.baseUrl}</p>
						</div>
						<div>
							<p class="text-sm text-[var(--color-muted-foreground)]">GitHub Organization</p>
							<p class="text-sm text-[var(--color-foreground)]">{systemSettings.githubOrg || 'Not configured'}</p>
						</div>
						<div>
							<p class="text-sm text-[var(--color-muted-foreground)]">Allowed Users</p>
							<p class="text-sm text-[var(--color-foreground)]">
								{systemSettings.githubAllowedUsers.length > 0
									? systemSettings.githubAllowedUsers.join(', ')
									: 'All authenticated users'}
							</p>
						</div>
						<div>
							<p class="text-sm text-[var(--color-muted-foreground)]">System Telegram</p>
							<span
								class="inline-flex items-center rounded-full px-2 py-1 text-xs font-medium {systemSettings.systemTelegramEnabled
									? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
									: 'bg-[var(--color-muted)] text-[var(--color-muted-foreground)]'}"
							>
								{systemSettings.systemTelegramEnabled ? 'Enabled' : 'Disabled'}
							</span>
						</div>
					</div>

					<div class="pt-4 border-t border-amber-500/30">
						<p class="text-sm text-[var(--color-muted-foreground)] mb-2">System Statistics</p>
						<div class="flex gap-8">
							<div>
								<p class="text-2xl font-bold text-[var(--color-foreground)]">{systemSettings.totalUsers}</p>
								<p class="text-xs text-[var(--color-muted-foreground)]">Total Users</p>
							</div>
							<div>
								<p class="text-2xl font-bold text-[var(--color-foreground)]">{systemSettings.totalEndpoints}</p>
								<p class="text-xs text-[var(--color-muted-foreground)]">Total Endpoints</p>
							</div>
						</div>
					</div>
				</div>
			</section>
		{/if}
	{/if}
</div>
