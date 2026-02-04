<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { edgeClientNoRedirect } from '$lib/api/client';
	import { theme } from '$lib/stores/theme';
	import ThemeSwitcher from '$lib/components/ThemeSwitcher.svelte';

	interface Props {
		children: import('svelte').Snippet;
	}

	let { children }: Props = $props();

	let user = $state<{ username: string; avatarUrl: string } | null>(null);
	let authChecked = $state(false);
	let userMenuOpen = $state(false);

	const navItems = [
		{ href: '/', label: 'Dashboard' },
		{ href: '/endpoints', label: 'Endpoints' },
		{ href: '/webhooks', label: 'Webhooks' },
		{ href: '/cli', label: 'CLI' },
		{ href: '/settings', label: 'Settings' }
	];

	const isLoginPage = $derived($page.url.pathname === '/login');
	const isHomePage = $derived($page.url.pathname === '/');
	const showLandingHeader = $derived(isHomePage && authChecked && !user);
	const showAppNav = $derived(!isLoginPage && !showLandingHeader);

	onMount(async () => {
		// Skip fetching settings on login page
		if (isLoginPage) return;

		try {
			const settings = await edgeClientNoRedirect.getSettings({});
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
		} finally {
			authChecked = true;
		}
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Hookly</title>
</svelte:head>

<div class="min-h-screen bg-[var(--color-background)]">
	{#if showLandingHeader}
		<!-- Minimal landing page header -->
		<header class="absolute top-0 left-0 right-0 z-10">
			<div class="flex h-16 items-center px-4 max-w-screen-xl mx-auto">
				<a href="/" class="flex items-center gap-2 font-semibold text-[var(--color-foreground)]">
					<span class="text-xl">🪝</span>
					<span>Hookly</span>
				</a>

				<div class="ml-auto flex items-center gap-3">
					<a
						href="https://github.com/dx314/hookly"
						target="_blank"
						rel="noopener noreferrer"
						class="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg border border-[var(--color-border)] text-sm text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)] hover:bg-[var(--color-muted)] transition-colors"
					>
						<svg class="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
							<path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
						</svg>
						<svg class="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
							<path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z" />
						</svg>
						<span>Star</span>
					</a>
					<ThemeSwitcher />
					<a
						href="/auth/login"
						class="inline-flex items-center justify-center gap-2 rounded-lg bg-[var(--color-foreground)] px-4 py-2 text-sm font-medium text-[var(--color-background)] hover:opacity-90 transition-opacity"
					>
						Sign in
					</a>
				</div>
			</div>
		</header>
	{:else if showAppNav}
		<!-- App navigation bar -->
		<header class="border-b border-[var(--color-border)] bg-[var(--color-background)]">
			<div class="flex h-14 items-center px-4 gap-4 max-w-screen-xl mx-auto">
				<a href="/" class="flex items-center gap-2 font-semibold text-[var(--color-foreground)]">
					<span class="text-xl">🪝</span>
					<span>Hookly</span>
				</a>

				<nav class="flex items-center gap-6 ml-8">
					{#each navItems as item (item.href)}
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
						<div class="relative">
							<button
								onclick={() => (userMenuOpen = !userMenuOpen)}
								class="flex items-center gap-2 rounded-lg px-2 py-1 hover:bg-[var(--color-muted)] transition-colors"
							>
								{#if user.avatarUrl}
									<img
										src={user.avatarUrl}
										alt={user.username}
										class="w-7 h-7 rounded-full"
									/>
								{/if}
								<span class="text-sm text-[var(--color-muted-foreground)]">{user.username}</span>
								<svg
									class="w-4 h-4 text-[var(--color-muted-foreground)]"
									fill="none"
									stroke="currentColor"
									viewBox="0 0 24 24"
								>
									<path
										stroke-linecap="round"
										stroke-linejoin="round"
										stroke-width="2"
										d="M19 9l-7 7-7-7"
									/>
								</svg>
							</button>
							{#if userMenuOpen}
								<!-- svelte-ignore a11y_no_static_element_interactions -->
								<div
									class="fixed inset-0 z-40"
									onclick={() => (userMenuOpen = false)}
									onkeydown={(e) => e.key === 'Escape' && (userMenuOpen = false)}
								></div>
								<div
									class="absolute right-0 mt-2 w-48 rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] shadow-lg py-1 z-50"
								>
									<button
										onclick={async () => {
											await fetch('/auth/logout', { method: 'POST' });
											window.location.href = '/';
										}}
										class="w-full px-4 py-2 text-left text-sm text-[var(--color-muted-foreground)] hover:bg-[var(--color-muted)] hover:text-[var(--color-foreground)] transition-colors"
									>
										Logout
									</button>
								</div>
							{/if}
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
	<main class="{isLoginPage || showLandingHeader ? '' : 'max-w-screen-xl mx-auto py-6 px-4'}">
		{@render children()}
	</main>
</div>
