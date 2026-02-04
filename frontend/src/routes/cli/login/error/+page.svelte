<script lang="ts">
	import { page } from '$app/stores';

	const error = $derived($page.url.searchParams.get('error') ?? 'An unknown error occurred');
</script>

<div class="flex min-h-[60vh] items-center justify-center">
	<div class="w-full max-w-sm space-y-6 text-center">
		<div class="flex items-center justify-center w-14 h-14 mx-auto rounded-full bg-red-500/10 border border-red-500/20">
			<svg class="w-7 h-7 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
				<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
			</svg>
		</div>

		<div class="space-y-2">
			<h1 class="text-xl font-semibold text-[var(--color-foreground)]">Login Failed</h1>
		</div>

		<div class="rounded-lg border border-red-500/20 bg-red-500/5 p-3">
			<p class="text-sm text-red-400">{error}</p>
		</div>

		<p class="text-sm text-[var(--color-muted-foreground)]">
			Please close this window and try again.
		</p>
	</div>
</div>
