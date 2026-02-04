<script lang="ts">
	let copied = $state<string | null>(null);
	let activeSection = $state('installation');

	function copyToClipboard(text: string, id: string) {
		navigator.clipboard.writeText(text);
		copied = id;
		setTimeout(() => (copied = null), 2000);
	}

	// Navigation sections for the sidebar
	const sections = [
		{ id: 'installation', label: 'Installation', icon: 'download' },
		{ id: 'quickstart', label: 'Quick Start', icon: 'rocket' },
		{ id: 'commands', label: 'Commands', icon: 'terminal' },
		{ id: 'service', label: 'Service', icon: 'server' },
		{ id: 'config', label: 'Configuration', icon: 'settings' },
		{ id: 'troubleshooting', label: 'Troubleshooting', icon: 'help' }
	];

	function scrollToSection(id: string) {
		activeSection = id;
		document.getElementById(id)?.scrollIntoView({ behavior: 'smooth' });
	}
</script>

<div class="flex gap-8">
	<!-- Sidebar Navigation -->
	<aside class="hidden lg:block w-48 flex-shrink-0">
		<nav class="sticky top-6 space-y-1">
			<h3 class="text-xs font-semibold text-[var(--color-muted-foreground)] uppercase tracking-wider mb-3">
				On this page
			</h3>
			{#each sections as section (section.title)}
				<button
					onclick={() => scrollToSection(section.id)}
					class="w-full text-left px-3 py-2 text-sm rounded-md transition-colors {activeSection === section.id
						? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)] font-medium'
						: 'text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)] hover:bg-[var(--color-muted)]/50'}"
				>
					{section.label}
				</button>
			{/each}
		</nav>
	</aside>

	<!-- Main Content -->
	<div class="flex-1 min-w-0 space-y-12 max-w-3xl">
		<!-- Header -->
		<div class="border-b border-[var(--color-border)] pb-6">
			<div class="flex items-center gap-3 mb-2">
				<div class="p-2 rounded-lg bg-[var(--color-primary)]/10">
					<svg class="w-6 h-6 text-[var(--color-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
					</svg>
				</div>
				<h1 class="text-2xl font-bold text-[var(--color-foreground)]">Hookly CLI</h1>
			</div>
			<p class="text-[var(--color-muted-foreground)]">
				Run the relay client on your home network to receive webhooks locally. No VPN required.
			</p>
		</div>

		<!-- Architecture Overview -->
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-muted)]/20 p-6">
			<h3 class="text-sm font-semibold text-[var(--color-foreground)] mb-4">How it works</h3>
			<div class="flex items-center justify-between text-sm gap-2 overflow-x-auto pb-2">
				<div class="flex flex-col items-center gap-1 min-w-fit">
					<div class="w-10 h-10 rounded-lg bg-blue-500/20 flex items-center justify-center">
						<svg class="w-5 h-5 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064" />
						</svg>
					</div>
					<span class="text-[var(--color-muted-foreground)]">Webhook</span>
					<span class="text-xs text-[var(--color-muted-foreground)]">Stripe, GitHub...</span>
				</div>
				<svg class="w-6 h-6 text-[var(--color-border)] flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6" />
				</svg>
				<div class="flex flex-col items-center gap-1 min-w-fit">
					<div class="w-10 h-10 rounded-lg bg-green-500/20 flex items-center justify-center">
						<svg class="w-5 h-5 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2" />
						</svg>
					</div>
					<span class="text-[var(--color-muted-foreground)]">Edge Server</span>
					<span class="text-xs text-[var(--color-muted-foreground)]">hooks.dx314.com</span>
				</div>
				<svg class="w-6 h-6 text-[var(--color-border)] flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6" />
				</svg>
				<div class="flex flex-col items-center gap-1 min-w-fit">
					<div class="w-10 h-10 rounded-lg bg-purple-500/20 flex items-center justify-center">
						<svg class="w-5 h-5 text-purple-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
						</svg>
					</div>
					<span class="text-[var(--color-muted-foreground)]">Hookly CLI</span>
					<span class="text-xs text-[var(--color-muted-foreground)]">Your machine</span>
				</div>
				<svg class="w-6 h-6 text-[var(--color-border)] flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6" />
				</svg>
				<div class="flex flex-col items-center gap-1 min-w-fit">
					<div class="w-10 h-10 rounded-lg bg-orange-500/20 flex items-center justify-center">
						<svg class="w-5 h-5 text-orange-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7c-2 0-3 1-3 3z" />
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-3-3v6" />
						</svg>
					</div>
					<span class="text-[var(--color-muted-foreground)]">Your App</span>
					<span class="text-xs text-[var(--color-muted-foreground)]">localhost:3000</span>
				</div>
			</div>
		</div>

		<!-- Installation -->
		<section id="installation" class="scroll-mt-6 space-y-4">
			<h2 class="text-xl font-semibold text-[var(--color-foreground)] flex items-center gap-2">
				<svg class="w-5 h-5 text-[var(--color-muted-foreground)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
				</svg>
				Installation
			</h2>

			<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] overflow-hidden">
				<div class="border-b border-[var(--color-border)] px-4 py-2 bg-[var(--color-muted)]/30 flex items-center justify-between">
					<span class="text-sm font-medium text-[var(--color-muted-foreground)]">Install with Go</span>
					<span class="text-xs text-[var(--color-muted-foreground)]">Requires Go 1.21+</span>
				</div>
				<div class="p-4 relative">
					<pre class="font-mono text-sm text-[var(--color-foreground)] overflow-x-auto">go install hooks.dx314.com/hookly@latest</pre>
					<button
						onclick={() => copyToClipboard('go install hooks.dx314.com/hookly@latest', 'go-install')}
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

			<div class="flex items-start gap-2 text-sm text-[var(--color-muted-foreground)]">
				<svg class="w-4 h-4 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
				</svg>
				<span>Binary releases coming soon for Linux, macOS, and Windows.</span>
			</div>
		</section>

		<!-- Quick Start -->
		<section id="quickstart" class="scroll-mt-6 space-y-4">
			<h2 class="text-xl font-semibold text-[var(--color-foreground)] flex items-center gap-2">
				<svg class="w-5 h-5 text-[var(--color-muted-foreground)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
				</svg>
				Quick Start
			</h2>

			<div class="space-y-4">
				<!-- Step 1 -->
				<div class="flex gap-4">
					<div class="flex flex-col items-center">
						<span class="w-8 h-8 rounded-full bg-[var(--color-primary)] flex items-center justify-center text-sm font-bold text-white">1</span>
						<div class="flex-1 w-px bg-[var(--color-border)] my-2"></div>
					</div>
					<div class="flex-1 pb-6">
						<h3 class="font-medium text-[var(--color-foreground)] mb-1">Authenticate</h3>
						<p class="text-sm text-[var(--color-muted-foreground)] mb-3">Log in with your GitHub account to link your CLI.</p>
						<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-muted)]/30 p-3 relative">
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

				<!-- Step 2 -->
				<div class="flex gap-4">
					<div class="flex flex-col items-center">
						<span class="w-8 h-8 rounded-full bg-[var(--color-primary)] flex items-center justify-center text-sm font-bold text-white">2</span>
						<div class="flex-1 w-px bg-[var(--color-border)] my-2"></div>
					</div>
					<div class="flex-1 pb-6">
						<h3 class="font-medium text-[var(--color-foreground)] mb-1">Configure</h3>
						<p class="text-sm text-[var(--color-muted-foreground)] mb-3">Create a config file by selecting an endpoint or creating a new one.</p>
						<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-muted)]/30 p-3 relative">
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
						<p class="mt-2 text-xs text-[var(--color-muted-foreground)]">Creates <code class="font-mono bg-[var(--color-muted)]/50 px-1 rounded">hookly.yaml</code> in your current directory</p>
					</div>
				</div>

				<!-- Step 3 -->
				<div class="flex gap-4">
					<div class="flex flex-col items-center">
						<span class="w-8 h-8 rounded-full bg-[var(--color-primary)] flex items-center justify-center text-sm font-bold text-white">3</span>
					</div>
					<div class="flex-1">
						<h3 class="font-medium text-[var(--color-foreground)] mb-1">Run</h3>
						<p class="text-sm text-[var(--color-muted-foreground)] mb-3">Start the relay and webhooks will be forwarded to your local service.</p>
						<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-muted)]/30 p-3 relative">
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
					</div>
				</div>
			</div>

			<!-- Success callout -->
			<div class="rounded-lg border border-green-500/30 bg-green-500/10 p-4 flex gap-3">
				<svg class="w-5 h-5 text-green-500 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
				</svg>
				<div class="text-sm">
					<p class="font-medium text-green-600 dark:text-green-400">You're all set!</p>
					<p class="text-green-600/80 dark:text-green-400/80">Webhooks sent to your endpoint URL will now be forwarded to your local machine.</p>
				</div>
			</div>
		</section>

		<!-- Commands Reference -->
		<section id="commands" class="scroll-mt-6 space-y-4">
			<h2 class="text-xl font-semibold text-[var(--color-foreground)] flex items-center gap-2">
				<svg class="w-5 h-5 text-[var(--color-muted-foreground)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
				</svg>
				Commands
			</h2>

			<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] overflow-hidden">
				<!-- Default command -->
				<div class="p-4 border-b border-[var(--color-border)] bg-[var(--color-muted)]/20">
					<div class="flex items-baseline gap-4">
						<code class="font-mono text-sm text-[var(--color-primary)] font-semibold">hookly</code>
						<span class="text-sm text-[var(--color-muted-foreground)]">Start the webhook relay (default command)</span>
					</div>
					<p class="mt-2 text-xs text-[var(--color-muted-foreground)]">Connects to the edge server and forwards webhooks to your local endpoints. Requires <code class="font-mono">hookly.yaml</code> and valid credentials.</p>
				</div>

				<!-- Auth commands group -->
				<div class="border-b border-[var(--color-border)]">
					<div class="px-4 py-2 bg-[var(--color-muted)]/30">
						<span class="text-xs font-semibold text-[var(--color-muted-foreground)] uppercase tracking-wider">Authentication</span>
					</div>
					<div class="divide-y divide-[var(--color-border)]">
						<div class="p-4 flex gap-4 hover:bg-[var(--color-muted)]/10 transition-colors">
							<code class="font-mono text-sm text-[var(--color-foreground)] w-40 flex-shrink-0">login</code>
							<span class="text-sm text-[var(--color-muted-foreground)]">Authenticate via browser OAuth, store encrypted token locally</span>
						</div>
						<div class="p-4 flex gap-4 hover:bg-[var(--color-muted)]/10 transition-colors">
							<code class="font-mono text-sm text-[var(--color-foreground)] w-40 flex-shrink-0">logout</code>
							<span class="text-sm text-[var(--color-muted-foreground)]">Clear stored credentials</span>
						</div>
						<div class="p-4 flex gap-4 hover:bg-[var(--color-muted)]/10 transition-colors">
							<code class="font-mono text-sm text-[var(--color-foreground)] w-40 flex-shrink-0">whoami</code>
							<span class="text-sm text-[var(--color-muted-foreground)]">Show current authenticated user and edge server</span>
						</div>
						<div class="p-4 flex gap-4 hover:bg-[var(--color-muted)]/10 transition-colors">
							<code class="font-mono text-sm text-[var(--color-foreground)] w-40 flex-shrink-0">status</code>
							<span class="text-sm text-[var(--color-muted-foreground)]">Show auth status, config details, and endpoint count</span>
						</div>
					</div>
				</div>

				<!-- Setup commands group -->
				<div class="border-b border-[var(--color-border)]">
					<div class="px-4 py-2 bg-[var(--color-muted)]/30">
						<span class="text-xs font-semibold text-[var(--color-muted-foreground)] uppercase tracking-wider">Setup</span>
					</div>
					<div class="p-4 flex gap-4 hover:bg-[var(--color-muted)]/10 transition-colors">
						<code class="font-mono text-sm text-[var(--color-foreground)] w-40 flex-shrink-0">init</code>
						<span class="text-sm text-[var(--color-muted-foreground)]">Interactive wizard to create <code class="font-mono bg-[var(--color-muted)]/50 px-1 rounded">hookly.yaml</code></span>
					</div>
				</div>

				<!-- Help commands group -->
				<div>
					<div class="px-4 py-2 bg-[var(--color-muted)]/30">
						<span class="text-xs font-semibold text-[var(--color-muted-foreground)] uppercase tracking-wider">Help</span>
					</div>
					<div class="divide-y divide-[var(--color-border)]">
						<div class="p-4 flex gap-4 hover:bg-[var(--color-muted)]/10 transition-colors">
							<code class="font-mono text-sm text-[var(--color-foreground)] w-40 flex-shrink-0">--help, -h</code>
							<span class="text-sm text-[var(--color-muted-foreground)]">Show help for any command</span>
						</div>
						<div class="p-4 flex gap-4 hover:bg-[var(--color-muted)]/10 transition-colors">
							<code class="font-mono text-sm text-[var(--color-foreground)] w-40 flex-shrink-0">--version, -v</code>
							<span class="text-sm text-[var(--color-muted-foreground)]">Print version information</span>
						</div>
					</div>
				</div>
			</div>

			<!-- Login options -->
			<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-4 space-y-3">
				<h3 class="text-sm font-semibold text-[var(--color-foreground)]">Login Options</h3>
				<div class="flex gap-4 items-baseline">
					<code class="font-mono text-sm text-[var(--color-foreground)] w-32 flex-shrink-0">--edge-url</code>
					<span class="text-sm text-[var(--color-muted-foreground)]">Connect to a custom edge server (default: <code class="font-mono bg-[var(--color-muted)]/50 px-1 rounded">https://hooks.dx314.com</code>)</span>
				</div>
				<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-muted)]/30 p-3 relative">
					<pre class="font-mono text-sm">hookly login --edge-url https://hooks.example.com</pre>
					<button
						onclick={() => copyToClipboard('hookly login --edge-url https://hooks.example.com', 'login-custom')}
						class="absolute top-2 right-2 p-1 rounded hover:bg-[var(--color-muted)] transition-colors text-[var(--color-muted-foreground)]"
					>
						{#if copied === 'login-custom'}
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
		</section>

		<!-- Service Management -->
		<section id="service" class="scroll-mt-6 space-y-4">
			<h2 class="text-xl font-semibold text-[var(--color-foreground)] flex items-center gap-2">
				<svg class="w-5 h-5 text-[var(--color-muted-foreground)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2" />
				</svg>
				Service Management
			</h2>

			<p class="text-sm text-[var(--color-muted-foreground)]">
				Install hookly as a background service to run automatically on boot (Linux/macOS).
			</p>

			<!-- Service commands table -->
			<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] divide-y divide-[var(--color-border)]">
				<div class="p-4 flex gap-4 hover:bg-[var(--color-muted)]/10 transition-colors">
					<code class="font-mono text-sm text-[var(--color-foreground)] w-40 flex-shrink-0">service install</code>
					<span class="text-sm text-[var(--color-muted-foreground)]">Register hookly as a system service</span>
				</div>
				<div class="p-4 flex gap-4 hover:bg-[var(--color-muted)]/10 transition-colors">
					<code class="font-mono text-sm text-[var(--color-foreground)] w-40 flex-shrink-0">service uninstall</code>
					<span class="text-sm text-[var(--color-muted-foreground)]">Remove the service registration</span>
				</div>
				<div class="p-4 flex gap-4 hover:bg-[var(--color-muted)]/10 transition-colors">
					<code class="font-mono text-sm text-[var(--color-foreground)] w-40 flex-shrink-0">service start</code>
					<span class="text-sm text-[var(--color-muted-foreground)]">Start the hookly service</span>
				</div>
				<div class="p-4 flex gap-4 hover:bg-[var(--color-muted)]/10 transition-colors">
					<code class="font-mono text-sm text-[var(--color-foreground)] w-40 flex-shrink-0">service stop</code>
					<span class="text-sm text-[var(--color-muted-foreground)]">Stop the hookly service</span>
				</div>
				<div class="p-4 flex gap-4 hover:bg-[var(--color-muted)]/10 transition-colors">
					<code class="font-mono text-sm text-[var(--color-foreground)] w-40 flex-shrink-0">service restart</code>
					<span class="text-sm text-[var(--color-muted-foreground)]">Restart the hookly service</span>
				</div>
				<div class="p-4 flex gap-4 hover:bg-[var(--color-muted)]/10 transition-colors">
					<code class="font-mono text-sm text-[var(--color-foreground)] w-40 flex-shrink-0">service status</code>
					<span class="text-sm text-[var(--color-muted-foreground)]">Check if the service is running</span>
				</div>
				<div class="p-4 flex gap-4 hover:bg-[var(--color-muted)]/10 transition-colors">
					<code class="font-mono text-sm text-[var(--color-foreground)] w-40 flex-shrink-0">service logs</code>
					<span class="text-sm text-[var(--color-muted-foreground)]">View service logs (<code class="font-mono">-f</code> to follow, <code class="font-mono">-n</code> for line count)</span>
				</div>
			</div>

			<!-- Two column layout for user/system service -->
			<div class="grid md:grid-cols-2 gap-4">
				<!-- User service -->
				<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-4 space-y-3">
					<div class="flex items-center gap-2">
						<svg class="w-4 h-4 text-[var(--color-muted-foreground)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
						</svg>
						<h3 class="text-sm font-semibold text-[var(--color-foreground)]">User Service</h3>
						<span class="text-xs px-2 py-0.5 rounded-full bg-green-500/20 text-green-600 dark:text-green-400">No sudo</span>
					</div>
					<p class="text-xs text-[var(--color-muted-foreground)]">Runs when you log in. Best for personal development machines.</p>
					<div class="rounded border border-[var(--color-border)] bg-[var(--color-muted)]/30 p-2 relative">
						<pre class="font-mono text-xs overflow-x-auto">hookly service install --user --config ./hookly.yaml
hookly service start --user</pre>
						<button
							onclick={() => copyToClipboard('hookly service install --user --config ./hookly.yaml && hookly service start --user', 'svc-user')}
							class="absolute top-1.5 right-1.5 p-1 rounded hover:bg-[var(--color-muted)] transition-colors text-[var(--color-muted-foreground)]"
						>
							{#if copied === 'svc-user'}
								<svg class="w-3 h-3 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
								</svg>
							{:else}
								<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
								</svg>
							{/if}
						</button>
					</div>
				</div>

				<!-- System service -->
				<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-4 space-y-3">
					<div class="flex items-center gap-2">
						<svg class="w-4 h-4 text-[var(--color-muted-foreground)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
						</svg>
						<h3 class="text-sm font-semibold text-[var(--color-foreground)]">System Service</h3>
						<span class="text-xs px-2 py-0.5 rounded-full bg-blue-500/20 text-blue-600 dark:text-blue-400">Requires sudo</span>
					</div>
					<p class="text-xs text-[var(--color-muted-foreground)]">Runs at system boot. Best for servers and always-on setups.</p>
					<div class="rounded border border-[var(--color-border)] bg-[var(--color-muted)]/30 p-2 relative">
						<pre class="font-mono text-xs overflow-x-auto">sudo hookly service install --config /etc/hookly/hookly.yaml
sudo hookly service start</pre>
						<button
							onclick={() => copyToClipboard('sudo hookly service install --config /etc/hookly/hookly.yaml && sudo hookly service start', 'svc-system')}
							class="absolute top-1.5 right-1.5 p-1 rounded hover:bg-[var(--color-muted)] transition-colors text-[var(--color-muted-foreground)]"
						>
							{#if copied === 'svc-system'}
								<svg class="w-3 h-3 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
								</svg>
							{:else}
								<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
								</svg>
							{/if}
						</button>
					</div>
				</div>
			</div>

			<!-- Platform note -->
			<div class="flex items-start gap-2 text-sm text-[var(--color-muted-foreground)]">
				<svg class="w-4 h-4 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
				</svg>
				<span>Uses <strong>systemd</strong> on Linux, <strong>launchd</strong> on macOS. Auto-restarts on failure.</span>
			</div>
		</section>

		<!-- Configuration -->
		<section id="config" class="scroll-mt-6 space-y-4">
			<h2 class="text-xl font-semibold text-[var(--color-foreground)] flex items-center gap-2">
				<svg class="w-5 h-5 text-[var(--color-muted-foreground)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
				</svg>
				Configuration
			</h2>

			<!-- Config file -->
			<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] overflow-hidden">
				<div class="border-b border-[var(--color-border)] px-4 py-2 bg-[var(--color-muted)]/30 flex items-center justify-between">
					<span class="text-sm font-medium text-[var(--color-muted-foreground)]">hookly.yaml</span>
					<span class="text-xs text-[var(--color-muted-foreground)]">Created by <code class="font-mono">hookly init</code></span>
				</div>
				<div class="p-4 relative">
					<pre class="font-mono text-sm text-[var(--color-foreground)] overflow-x-auto"><span class="text-[var(--color-muted-foreground)]"># Edge server to connect to</span>
edge_url: "https://hooks.dx314.com"

<span class="text-[var(--color-muted-foreground)]"># Hub ID (optional - auto-generated from hostname)</span>
hub_id: "my-laptop"

<span class="text-[var(--color-muted-foreground)]"># Endpoints to relay</span>
endpoints:
  - id: "ep_abc123xyz"
    destination: "http://localhost:3000/webhooks/stripe"
  - id: "ep_def456uvw"
    <span class="text-[var(--color-muted-foreground)]"># Uses edge-configured destination</span></pre>
				</div>
			</div>

			<!-- File locations -->
			<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] divide-y divide-[var(--color-border)]">
				<div class="px-4 py-2 bg-[var(--color-muted)]/30">
					<span class="text-xs font-semibold text-[var(--color-muted-foreground)] uppercase tracking-wider">File Locations</span>
				</div>
				<div class="p-4 flex gap-4">
					<code class="font-mono text-sm text-[var(--color-foreground)] flex-shrink-0">./hookly.yaml</code>
					<span class="text-sm text-[var(--color-muted-foreground)]">Endpoint configuration (current directory)</span>
				</div>
				<div class="p-4 flex gap-4">
					<code class="font-mono text-sm text-[var(--color-foreground)] flex-shrink-0">~/.config/hookly/credentials.json</code>
					<span class="text-sm text-[var(--color-muted-foreground)]">Encrypted auth credentials</span>
				</div>
			</div>
		</section>

		<!-- Troubleshooting -->
		<section id="troubleshooting" class="scroll-mt-6 space-y-4">
			<h2 class="text-xl font-semibold text-[var(--color-foreground)] flex items-center gap-2">
				<svg class="w-5 h-5 text-[var(--color-muted-foreground)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.228 9c.549-1.165 2.03-2 3.772-2 2.21 0 4 1.343 4 3 0 1.4-1.278 2.575-3.006 2.907-.542.104-.994.54-.994 1.093m0 3h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
				</svg>
				Troubleshooting
			</h2>

			<div class="space-y-4">
				<!-- Issue 1 -->
				<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-4">
					<h3 class="font-medium text-[var(--color-foreground)] mb-2 flex items-center gap-2">
						<span class="text-red-500">✕</span>
						"Not logged in" error
					</h3>
					<p class="text-sm text-[var(--color-muted-foreground)] mb-3">Your credentials are missing or expired.</p>
					<div class="rounded border border-[var(--color-border)] bg-[var(--color-muted)]/30 p-2">
						<code class="font-mono text-sm">hookly login</code>
					</div>
				</div>

				<!-- Issue 2 -->
				<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-4">
					<h3 class="font-medium text-[var(--color-foreground)] mb-2 flex items-center gap-2">
						<span class="text-red-500">✕</span>
						"Endpoint not found" error
					</h3>
					<p class="text-sm text-[var(--color-muted-foreground)] mb-3">The endpoint in your config was deleted or the ID is incorrect.</p>
					<div class="rounded border border-[var(--color-border)] bg-[var(--color-muted)]/30 p-2">
						<code class="font-mono text-sm">rm hookly.yaml && hookly init</code>
					</div>
				</div>

				<!-- Issue 3 -->
				<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-4">
					<h3 class="font-medium text-[var(--color-foreground)] mb-2 flex items-center gap-2">
						<span class="text-red-500">✕</span>
						"Access denied" error
					</h3>
					<p class="text-sm text-[var(--color-muted-foreground)] mb-3">You're trying to use an endpoint owned by another user.</p>
					<div class="rounded border border-[var(--color-border)] bg-[var(--color-muted)]/30 p-2">
						<code class="font-mono text-sm">hookly whoami  <span class="text-[var(--color-muted-foreground)]"># check which user you're logged in as</span></code>
					</div>
				</div>

				<!-- Issue 4 -->
				<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-4">
					<h3 class="font-medium text-[var(--color-foreground)] mb-2 flex items-center gap-2">
						<span class="text-red-500">✕</span>
						Service permission denied
					</h3>
					<p class="text-sm text-[var(--color-muted-foreground)] mb-3">System services require root privileges.</p>
					<div class="rounded border border-[var(--color-border)] bg-[var(--color-muted)]/30 p-2">
						<code class="font-mono text-sm">sudo hookly service install --config ...  <span class="text-[var(--color-muted-foreground)]"># or use --user</span></code>
					</div>
				</div>

				<!-- Issue 5 -->
				<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-4">
					<h3 class="font-medium text-[var(--color-foreground)] mb-2 flex items-center gap-2">
						<span class="text-yellow-500">⚠</span>
						Webhooks not arriving
					</h3>
					<p class="text-sm text-[var(--color-muted-foreground)] mb-3">Check that your local service is running and accessible.</p>
					<div class="rounded border border-[var(--color-border)] bg-[var(--color-muted)]/30 p-2 space-y-1">
						<code class="font-mono text-sm block">hookly status  <span class="text-[var(--color-muted-foreground)]"># verify connection</span></code>
						<code class="font-mono text-sm block">curl http://localhost:3000/health  <span class="text-[var(--color-muted-foreground)]"># test local service</span></code>
					</div>
				</div>
			</div>
		</section>

		<!-- Footer -->
		<div class="border-t border-[var(--color-border)] pt-6 text-sm text-[var(--color-muted-foreground)]">
			<p>Need help? Check the <a href="https://github.com/alexdunmow/hookly" class="text-[var(--color-primary)] hover:underline">GitHub repository</a> or open an issue.</p>
		</div>
	</div>
</div>
