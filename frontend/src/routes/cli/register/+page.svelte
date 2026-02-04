<script lang="ts">
	import { page } from '$app/stores';

	const port = $derived($page.url.searchParams.get('port') ?? '');
	const state = $derived($page.url.searchParams.get('state') ?? '');
	const username = $derived($page.url.searchParams.get('username') ?? '');

	const missingParams = $derived(!port || !state || !username);
</script>

<div class="flex min-h-[60vh] items-center justify-center">
	<div class="w-full max-w-sm space-y-6">
		{#if missingParams}
			<div class="rounded-lg border border-[var(--color-destructive)] bg-[var(--color-destructive)]/10 p-4">
				<p class="text-[var(--color-destructive)]">Missing required parameters. Please try logging in again from the CLI.</p>
			</div>
		{:else}
			<div class="text-center space-y-2">
				<div class="flex items-center justify-center w-14 h-14 mx-auto rounded-lg bg-[var(--color-muted)] border border-[var(--color-border)]">
					<svg class="w-7 h-7 text-[var(--color-foreground)]" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
						<path stroke-linecap="round" stroke-linejoin="round" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
					</svg>
				</div>
				<h1 class="text-xl font-semibold text-[var(--color-foreground)]">Authorize CLI</h1>
				<p class="text-sm text-[var(--color-muted-foreground)]">
					Signed in as <span class="font-medium text-[var(--color-foreground)]">{username}</span>
				</p>
			</div>

			<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] divide-y divide-[var(--color-border)]">
				<div class="p-4 flex justify-between items-center">
					<span class="text-sm text-[var(--color-muted-foreground)]">Application</span>
					<span class="text-sm font-medium text-[var(--color-foreground)]">Hookly CLI</span>
				</div>
				<div class="p-4 flex justify-between items-center">
					<span class="text-sm text-[var(--color-muted-foreground)]">Access</span>
					<span class="text-sm font-medium text-[var(--color-foreground)]">Full API access</span>
				</div>
			</div>

			<form method="POST" action="/auth/cli/authorize" class="space-y-3">
				<input type="hidden" name="port" value={port}>
				<input type="hidden" name="state" value={state}>
				<button
					type="submit"
					class="w-full rounded-lg bg-[var(--color-foreground)] px-4 py-2.5 text-sm font-medium text-[var(--color-background)] hover:opacity-90 transition-opacity"
				>
					Authorize CLI
				</button>
			</form>

			<p class="text-center">
				<a href="/" class="text-sm text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)] transition-colors">
					Cancel
				</a>
			</p>
		{/if}
	</div>
</div>
