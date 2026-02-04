<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { page } from '$app/stores';

	interface Props {
		children: import('svelte').Snippet;
	}

	let { children }: Props = $props();

	const navItems = [
		{ href: '/', label: 'Dashboard' },
		{ href: '/endpoints', label: 'Endpoints' },
		{ href: '/webhooks', label: 'Webhooks' },
		{ href: '/settings', label: 'Settings' }
	];
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Hookly</title>
</svelte:head>

<div class="min-h-screen bg-[var(--color-background)]">
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
				<a
					href="/auth/login"
					class="text-sm font-medium text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)] transition-colors"
				>
					Login with GitHub
				</a>
			</div>
		</div>
	</header>

	<!-- Main content -->
	<main class="max-w-screen-xl mx-auto py-6 px-4">
		{@render children()}
	</main>
</div>
