<script lang="ts">
	import { page } from '$app/stores';

	const username = $derived($page.url.searchParams.get('username') ?? 'User');
</script>

<div class="flex min-h-[60vh] items-center justify-center">
	<div class="w-full max-w-sm space-y-6 text-center">
		<div class="flex items-center justify-center w-14 h-14 mx-auto rounded-full bg-green-500/10 border border-green-500/20">
			<svg class="w-7 h-7 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
				<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
			</svg>
		</div>

		<div class="space-y-2">
			<h1 class="text-xl font-semibold text-[var(--color-foreground)]">You're all set!</h1>
			<p class="text-sm text-[var(--color-muted-foreground)]">
				Logged in as <span class="font-medium text-[var(--color-foreground)]">{username}</span>
			</p>
		</div>

		<p class="text-sm text-[var(--color-muted-foreground)]">
			Your CLI is now connected to Hookly.<br>You can close this window.
		</p>

		<p class="text-xs text-[var(--color-muted-foreground)]/60">
			This window will close automatically...
		</p>
	</div>
</div>

<svelte:head>
	<script>
		setTimeout(function() { window.close(); }, 2000);
	</script>
</svelte:head>
