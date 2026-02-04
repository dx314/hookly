<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { edgeClient } from '$lib/api/client';
	import { theme } from '$lib/stores/theme';
	import ThemeSwitcher from '$lib/components/ThemeSwitcher.svelte';

	interface Props {
		children: import('svelte').Snippet;
	}

	let { children }: Props = $props();

	let user = $state<{ username: string; avatarUrl: string } | null>(null);

	const navItems = [
		{ href: '/', label: 'Dashboard' },
		{ href: '/endpoints', label: 'Endpoints' },
		{ href: '/webhooks', label: 'Webhooks' },
		{ href: '/cli', label: 'CLI' },
		{ href: '/settings', label: 'Settings' }
	];

	const isLoginPage = $derived($page.url.pathname === '/login');

	onMount(async () => {
		// Skip fetching settings on login page
		if (isLoginPage) return;

		try {
			const settings = await edgeClient.getSettings({});
			// Initialize theme from server
			theme.initialize(settings.themePreference);
			// Set user info
			if (settings.userId) {
				user = {
					username: settings.username,
					avatarUrl: settings.avatarUrl
				};
			}
		} catch {
			// User not logged in or error - theme will default to system
		}
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Hookly</title>
</svelte:head>

<div class="min-h-screen bg-[var(--color-background)]">
	{#if !isLoginPage}
		<!-- Top navigation bar -->
		<header class="border-b border-[var(--color-border)] bg-[var(--color-background)]">
			<div class="flex h-14 items-center px-4 gap-4 max-w-screen-xl mx-auto">
				<a href="/" class="flex items-center gap-2 font-semibold text-[var(--color-foreground)]">
					<span class="text-xl">🪝</span>
					<span>Hookly</span>
				</a>

				<nav class="flex items-center gap-6 ml-8">
					{#each navItems as item}
						<a
							href={item.href}
							class="text-sm font-medium transition-colors hover:text-[var(--color-foreground)] {$page.url.pathname === item.href || ($page.url.pathname.startsWith(item.href) && item.href !== '/') ? 'text-[var(--color-foreground)]' : 'text-[var(--color-muted-foreground)]'}"
						>
							{item.label}
						</a>
					{/each}
				</nav>

				<div class="ml-auto flex items-center gap-4">
					<ThemeSwitcher />
					{#if user}
						<div class="flex items-center gap-2">
							{#if user.avatarUrl}
								<img
									src={user.avatarUrl}
									alt={user.username}
									class="w-7 h-7 rounded-full"
								/>
							{/if}
							<span class="text-sm text-[var(--color-muted-foreground)]">{user.username}</span>
						</div>
					{:else}
						<a
							href="/auth/login"
							class="text-sm font-medium text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)] transition-colors"
						>
							Login with GitHub
						</a>
					{/if}
				</div>
			</div>
		</header>
	{/if}

	<!-- Main content -->
	<main class="{isLoginPage ? '' : 'max-w-screen-xl mx-auto py-6 px-4'}">
		{@render children()}
	</main>
</div>
