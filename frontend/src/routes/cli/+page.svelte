<script lang="ts">
	let copied = $state<string | null>(null);

	function copyToClipboard(text: string, id: string) {
		navigator.clipboard.writeText(text);
		copied = id;
		setTimeout(() => (copied = null), 2000);
	}
</script>

<div class="space-y-8 max-w-3xl">
	<div>
		<h1 class="text-2xl font-bold text-[var(--color-foreground)]">Install Hookly CLI</h1>
		<p class="text-[var(--color-muted-foreground)] mt-1">
			Run the relay client on your home network to receive webhooks locally.
		</p>
	</div>

	<!-- Installation -->
	<section class="space-y-4">
		<h2 class="text-lg font-semibold text-[var(--color-foreground)]">Installation</h2>

		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] overflow-hidden">
			<div class="border-b border-[var(--color-border)] px-4 py-2 bg-[var(--color-muted)]/30">
				<span class="text-sm font-medium text-[var(--color-muted-foreground)]">Go Install</span>
			</div>
			<div class="p-4 relative">
				<pre class="font-mono text-sm text-[var(--color-foreground)] overflow-x-auto">go install github.com/alexdunmow/hookly/cmd/hookly@latest</pre>
				<button
					onclick={() => copyToClipboard('go install github.com/alexdunmow/hookly/cmd/hookly@latest', 'go-install')}
					class="absolute top-3 right-3 p-1.5 rounded hover:bg-[var(--color-muted)] transition-colors text-[var(--color-muted-foreground)]"
					title="Copy to clipboard"
				>
					{#if copied === 'go-install'}
						<svg class="w-4 h-4 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
						</svg>
					{:else}
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
						</svg>
					{/if}
				</button>
			</div>
		</div>

		<p class="text-sm text-[var(--color-muted-foreground)]">
			Requires Go 1.21+. Binary releases coming soon.
		</p>
	</section>

	<!-- Quick Start -->
	<section class="space-y-4">
		<h2 class="text-lg font-semibold text-[var(--color-foreground)]">Quick Start</h2>

		<div class="space-y-3">
			<div class="flex items-start gap-3">
				<span class="flex-shrink-0 w-6 h-6 rounded-full bg-[var(--color-muted)] flex items-center justify-center text-xs font-medium text-[var(--color-muted-foreground)]">1</span>
				<div class="flex-1">
					<p class="text-sm text-[var(--color-foreground)]">Log in with your GitHub account</p>
					<div class="mt-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-muted)]/30 p-3 relative">
						<code class="font-mono text-sm">hookly login</code>
						<button
							onclick={() => copyToClipboard('hookly login', 'login')}
							class="absolute top-2 right-2 p-1 rounded hover:bg-[var(--color-muted)] transition-colors text-[var(--color-muted-foreground)]"
						>
							{#if copied === 'login'}
								<svg class="w-3.5 h-3.5 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
								</svg>
							{:else}
								<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
								</svg>
							{/if}
						</button>
					</div>
				</div>
			</div>

			<div class="flex items-start gap-3">
				<span class="flex-shrink-0 w-6 h-6 rounded-full bg-[var(--color-muted)] flex items-center justify-center text-xs font-medium text-[var(--color-muted-foreground)]">2</span>
				<div class="flex-1">
					<p class="text-sm text-[var(--color-foreground)]">Initialize config with your endpoint</p>
					<div class="mt-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-muted)]/30 p-3 relative">
						<code class="font-mono text-sm">hookly init</code>
						<button
							onclick={() => copyToClipboard('hookly init', 'init')}
							class="absolute top-2 right-2 p-1 rounded hover:bg-[var(--color-muted)] transition-colors text-[var(--color-muted-foreground)]"
						>
							{#if copied === 'init'}
								<svg class="w-3.5 h-3.5 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
								</svg>
							{:else}
								<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
								</svg>
							{/if}
						</button>
					</div>
					<p class="mt-1 text-xs text-[var(--color-muted-foreground)]">Creates <code class="font-mono">hookly.yaml</code> with your selected endpoint</p>
				</div>
			</div>

			<div class="flex items-start gap-3">
				<span class="flex-shrink-0 w-6 h-6 rounded-full bg-[var(--color-muted)] flex items-center justify-center text-xs font-medium text-[var(--color-muted-foreground)]">3</span>
				<div class="flex-1">
					<p class="text-sm text-[var(--color-foreground)]">Start the relay client</p>
					<div class="mt-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-muted)]/30 p-3 relative">
						<code class="font-mono text-sm">hookly</code>
						<button
							onclick={() => copyToClipboard('hookly', 'run')}
							class="absolute top-2 right-2 p-1 rounded hover:bg-[var(--color-muted)] transition-colors text-[var(--color-muted-foreground)]"
						>
							{#if copied === 'run'}
								<svg class="w-3.5 h-3.5 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
								</svg>
							{:else}
								<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
								</svg>
							{/if}
						</button>
					</div>
					<p class="mt-1 text-xs text-[var(--color-muted-foreground)]">Webhooks will now be forwarded to your local service</p>
				</div>
			</div>
		</div>
	</section>

	<!-- Command Reference -->
	<section class="space-y-4">
		<h2 class="text-lg font-semibold text-[var(--color-foreground)]">Commands</h2>

		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] divide-y divide-[var(--color-border)]">
			<div class="p-4 flex gap-4">
				<code class="font-mono text-sm text-[var(--color-foreground)] w-36 flex-shrink-0">hookly</code>
				<span class="text-sm text-[var(--color-muted-foreground)]">Run the relay client (default)</span>
			</div>
			<div class="p-4 flex gap-4">
				<code class="font-mono text-sm text-[var(--color-foreground)] w-36 flex-shrink-0">hookly login</code>
				<span class="text-sm text-[var(--color-muted-foreground)]">Authenticate via browser OAuth, store token locally</span>
			</div>
			<div class="p-4 flex gap-4">
				<code class="font-mono text-sm text-[var(--color-foreground)] w-36 flex-shrink-0">hookly logout</code>
				<span class="text-sm text-[var(--color-muted-foreground)]">Clear local token and revoke server session</span>
			</div>
			<div class="p-4 flex gap-4">
				<code class="font-mono text-sm text-[var(--color-foreground)] w-36 flex-shrink-0">hookly whoami</code>
				<span class="text-sm text-[var(--color-muted-foreground)]">Show current authenticated user</span>
			</div>
			<div class="p-4 flex gap-4">
				<code class="font-mono text-sm text-[var(--color-foreground)] w-36 flex-shrink-0">hookly status</code>
				<span class="text-sm text-[var(--color-muted-foreground)]">Show user, edge URL, and relay connection status</span>
			</div>
			<div class="p-4 flex gap-4">
				<code class="font-mono text-sm text-[var(--color-foreground)] w-36 flex-shrink-0">hookly init</code>
				<span class="text-sm text-[var(--color-muted-foreground)]">Interactive wizard to create <code class="font-mono">hookly.yaml</code></span>
			</div>
			<div class="p-4 flex gap-4">
				<code class="font-mono text-sm text-[var(--color-foreground)] w-36 flex-shrink-0">hookly version</code>
				<span class="text-sm text-[var(--color-muted-foreground)]">Print version information</span>
			</div>
			<div class="p-4 flex gap-4">
				<code class="font-mono text-sm text-[var(--color-foreground)] w-36 flex-shrink-0">hookly help</code>
				<span class="text-sm text-[var(--color-muted-foreground)]">Show help for commands</span>
			</div>
		</div>
	</section>

	<!-- Options -->
	<section class="space-y-4">
		<h2 class="text-lg font-semibold text-[var(--color-foreground)]">Options</h2>

		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] divide-y divide-[var(--color-border)]">
			<div class="p-4 flex gap-4">
				<code class="font-mono text-sm text-[var(--color-foreground)] w-36 flex-shrink-0">--edge-url</code>
				<span class="text-sm text-[var(--color-muted-foreground)]">Override edge gateway URL (default: https://hooks.dx314.com)</span>
			</div>
		</div>
	</section>

	<!-- Config File -->
	<section class="space-y-4">
		<h2 class="text-lg font-semibold text-[var(--color-foreground)]">Configuration</h2>

		<p class="text-sm text-[var(--color-muted-foreground)]">
			The <code class="font-mono">hookly init</code> command creates a <code class="font-mono">hookly.yaml</code> file in your current directory:
		</p>

		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] overflow-hidden">
			<div class="border-b border-[var(--color-border)] px-4 py-2 bg-[var(--color-muted)]/30">
				<span class="text-sm font-medium text-[var(--color-muted-foreground)]">hookly.yaml</span>
			</div>
			<div class="p-4">
				<pre class="font-mono text-sm text-[var(--color-foreground)] overflow-x-auto">endpoint_id: ep_abc123xyz
forward_url: http://localhost:8080/webhooks</pre>
			</div>
		</div>

		<p class="text-sm text-[var(--color-muted-foreground)]">
			Credentials are stored in <code class="font-mono">~/.config/hookly/</code>.
		</p>
	</section>
</div>
