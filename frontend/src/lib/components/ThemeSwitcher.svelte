<script lang="ts">
	import { theme, stringToThemePreference, type Theme } from '$lib/stores/theme';
	import { edgeClient } from '$lib/api/client';

	let isOpen = $state(false);
	let saving = $state(false);

	const themes: { value: Theme; label: string; icon: string }[] = [
		{ value: 'system', label: 'System', icon: '💻' },
		{ value: 'light', label: 'Light', icon: '☀️' },
		{ value: 'dark', label: 'Dark', icon: '🌙' },
		{ value: 'placid-blue-light', label: 'Blue Light', icon: '🌊' },
		{ value: 'placid-blue-dark', label: 'Blue Dark', icon: '🌌' }
	];

	function getCurrentIcon(): string {
		return themes.find((t) => t.value === $theme)?.icon ?? '💻';
	}

	async function selectTheme(newTheme: Theme) {
		if (saving) return;

		const oldTheme = $theme;
		theme.set(newTheme);
		isOpen = false;

		try {
			saving = true;
			await edgeClient.updateUserSettings({
				themePreference: stringToThemePreference(newTheme)
			});
		} catch (err) {
			// Revert on error
			console.error('Failed to save theme preference:', err);
			theme.set(oldTheme);
		} finally {
			saving = false;
		}
	}

	function handleClickOutside(event: MouseEvent) {
		const target = event.target as HTMLElement;
		if (!target.closest('.theme-switcher')) {
			isOpen = false;
		}
	}
</script>

<svelte:window onclick={handleClickOutside} />

<div class="theme-switcher relative">
	<button
		onclick={() => (isOpen = !isOpen)}
		class="flex items-center justify-center w-8 h-8 rounded-md hover:bg-[var(--color-muted)] transition-colors"
		aria-label="Change theme"
		title="Change theme"
	>
		<span class="text-sm">{getCurrentIcon()}</span>
	</button>

	{#if isOpen}
		<div
			class="absolute right-0 mt-2 w-40 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] shadow-lg z-50"
		>
			{#each themes as t (t.value)}
				<button
					onclick={() => selectTheme(t.value)}
					class="flex items-center gap-2 w-full px-3 py-2 text-sm text-left hover:bg-[var(--color-muted)] transition-colors first:rounded-t-md last:rounded-b-md {$theme === t.value ? 'bg-[var(--color-muted)]' : ''}"
					disabled={saving}
				>
					<span>{t.icon}</span>
					<span class="text-[var(--color-foreground)]">{t.label}</span>
					{#if $theme === t.value}
						<span class="ml-auto text-[var(--color-primary)]">✓</span>
					{/if}
				</button>
			{/each}
		</div>
	{/if}
</div>
