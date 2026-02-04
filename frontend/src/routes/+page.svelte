<script lang="ts">
	import { onMount } from 'svelte';
	import { edgeClientNoRedirect } from '$lib/api/client';
	import type { ConnectedEndpoint } from '$api/hookly/v1/common_pb';

	let status = $state<{
		pendingCount: number;
		failedCount: number;
		deadLetterCount: number;
		connectedEndpoints: ConnectedEndpoint[];
	} | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let isLoggedIn = $state(false);

	onMount(async () => {
		try {
			// Use non-redirecting client to check auth status
			const response = await edgeClientNoRedirect.getStatus({});
			isLoggedIn = true;
			status = {
				pendingCount: response.status?.pendingCount ?? 0,
				failedCount: response.status?.failedCount ?? 0,
				deadLetterCount: response.status?.deadLetterCount ?? 0,
				connectedEndpoints: response.status?.connectedEndpoints ?? []
			};
		} catch {
			// Not logged in - show landing page
			isLoggedIn = false;
		} finally {
			loading = false;
		}
	});
</script>

{#if loading}
	<!-- Loading state -->
	<div class="flex items-center justify-center min-h-[60vh]">
		<div class="animate-pulse text-[var(--color-muted-foreground)]">Loading...</div>
	</div>
{:else if !isLoggedIn}
	<!-- Marketing Landing Page for logged-out users -->
	<div class="min-h-screen">
		<!-- Hero Section -->
		<div class="relative overflow-hidden">
			<!-- Gradient background -->
			<div class="absolute inset-0 bg-gradient-to-br from-[var(--color-primary)]/5 via-transparent to-[var(--color-primary)]/10 pointer-events-none"></div>

			<div class="max-w-screen-xl mx-auto px-4 pt-20 pb-32">
				<div class="text-center max-w-3xl mx-auto">
					<div class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-[var(--color-muted)] text-sm text-[var(--color-muted-foreground)] mb-6">
						<span class="flex h-2 w-2 rounded-full bg-green-500 animate-pulse"></span>
						Now in public beta
					</div>

					<h1 class="text-5xl sm:text-6xl font-bold text-[var(--color-foreground)] tracking-tight">
						Webhooks to your
						<span class="bg-gradient-to-r from-blue-500 to-violet-500 bg-clip-text text-transparent">local machine</span>
					</h1>

					<p class="mt-6 text-xl text-[var(--color-muted-foreground)] max-w-2xl mx-auto">
						Receive webhooks from Stripe, GitHub, and other services directly on your development machine. No ngrok. No port forwarding. No VPN.
					</p>

					<div class="mt-10 flex flex-col sm:flex-row items-center justify-center gap-4">
						<a
							href="/auth/login"
							class="inline-flex items-center justify-center gap-2 rounded-lg bg-[var(--color-foreground)] px-6 py-3 text-base font-medium text-[var(--color-background)] hover:opacity-90 transition-opacity"
						>
							<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
								<path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
							</svg>
							Get Started Free
						</a>
						<a
							href="#how-it-works"
							class="inline-flex items-center justify-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] px-6 py-3 text-base font-medium text-[var(--color-foreground)] hover:bg-[var(--color-muted)] transition-colors"
						>
							See how it works
							<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
							</svg>
						</a>
					</div>

					<!-- Terminal preview -->
					<div class="mt-16 rounded-xl border border-[var(--color-border)] bg-[#0a0a0a] text-left overflow-hidden shadow-2xl">
						<div class="flex items-center gap-2 px-4 py-3 bg-[#1a1a1a] border-b border-[#333]">
							<span class="h-3 w-3 rounded-full bg-[#ff5f56]"></span>
							<span class="h-3 w-3 rounded-full bg-[#ffbd2e]"></span>
							<span class="h-3 w-3 rounded-full bg-[#27ca40]"></span>
							<span class="ml-4 text-sm text-[#888]">Terminal</span>
						</div>
						<div class="p-6 font-mono text-sm text-[#e5e5e5] space-y-2">
							<div><span class="text-[#888]">$</span> hookly login</div>
							<div class="text-green-400">Authenticated as alex via GitHub</div>
							<div class="mt-4"><span class="text-[#888]">$</span> hookly</div>
							<div class="text-[#888]">Connecting to hooks.dx314.com...</div>
							<div class="text-green-400">Connected! Listening for webhooks...</div>
							<div class="mt-4 text-blue-400">← stripe-payments: POST /webhook (200 OK)</div>
							<div class="text-blue-400">← github-events: POST /api/hooks/github (200 OK)</div>
							<div class="text-yellow-400">← telegram-bot: retrying in 2s...</div>
							<div class="text-green-400">← telegram-bot: POST /bot/update (200 OK)</div>
						</div>
					</div>
				</div>
			</div>
		</div>

		<!-- How it works -->
		<div id="how-it-works" class="py-24 bg-[var(--color-muted)]/30">
			<div class="max-w-screen-xl mx-auto px-4">
				<div class="text-center mb-16">
					<h2 class="text-3xl font-bold text-[var(--color-foreground)]">How it works</h2>
					<p class="mt-4 text-lg text-[var(--color-muted-foreground)]">Three steps to receive webhooks locally</p>
				</div>

				<div class="grid md:grid-cols-3 gap-8">
					<div class="relative p-8 rounded-xl border border-[var(--color-border)] bg-[var(--color-background)]">
						<div class="absolute -top-4 left-8 px-3 py-1 rounded-full bg-[var(--color-foreground)] text-[var(--color-background)] text-sm font-bold">1</div>
						<div class="text-4xl mb-4">🔗</div>
						<h3 class="text-xl font-semibold text-[var(--color-foreground)] mb-2">Create an endpoint</h3>
						<p class="text-[var(--color-muted-foreground)]">
							Get a unique public URL for each webhook provider. Configure signature verification for Stripe, GitHub, or custom schemes.
						</p>
					</div>

					<div class="relative p-8 rounded-xl border border-[var(--color-border)] bg-[var(--color-background)]">
						<div class="absolute -top-4 left-8 px-3 py-1 rounded-full bg-[var(--color-foreground)] text-[var(--color-background)] text-sm font-bold">2</div>
						<div class="text-4xl mb-4">⚡</div>
						<h3 class="text-xl font-semibold text-[var(--color-foreground)] mb-2">Run the CLI</h3>
						<p class="text-[var(--color-muted-foreground)]">
							Install the CLI and run <code class="px-1.5 py-0.5 rounded bg-[var(--color-muted)] text-sm">hookly</code>. It connects to our edge and streams webhooks to your local server.
						</p>
					</div>

					<div class="relative p-8 rounded-xl border border-[var(--color-border)] bg-[var(--color-background)]">
						<div class="absolute -top-4 left-8 px-3 py-1 rounded-full bg-[var(--color-foreground)] text-[var(--color-background)] text-sm font-bold">3</div>
						<div class="text-4xl mb-4">🎯</div>
						<h3 class="text-xl font-semibold text-[var(--color-foreground)] mb-2">Develop locally</h3>
						<p class="text-[var(--color-muted-foreground)]">
							Webhooks hit your localhost instantly. Failed deliveries retry automatically. View history and replay any webhook.
						</p>
					</div>
				</div>
			</div>
		</div>

		<!-- Features -->
		<div class="py-24">
			<div class="max-w-screen-xl mx-auto px-4">
				<div class="text-center mb-16">
					<h2 class="text-3xl font-bold text-[var(--color-foreground)]">Built for developers</h2>
					<p class="mt-4 text-lg text-[var(--color-muted-foreground)]">Everything you need for reliable webhook development</p>
				</div>

				<div class="grid md:grid-cols-2 lg:grid-cols-3 gap-6">
					<div class="p-6 rounded-xl border border-[var(--color-border)] bg-[var(--color-background)]">
						<div class="w-10 h-10 rounded-lg bg-blue-500/10 flex items-center justify-center text-blue-500 mb-4">
							<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
							</svg>
						</div>
						<h3 class="text-lg font-semibold text-[var(--color-foreground)] mb-2">Signature verification</h3>
						<p class="text-sm text-[var(--color-muted-foreground)]">
							Built-in verification for Stripe, GitHub, Telegram, and custom HMAC schemes. Reject invalid webhooks at the edge.
						</p>
					</div>

					<div class="p-6 rounded-xl border border-[var(--color-border)] bg-[var(--color-background)]">
						<div class="w-10 h-10 rounded-lg bg-green-500/10 flex items-center justify-center text-green-500 mb-4">
							<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
							</svg>
						</div>
						<h3 class="text-lg font-semibold text-[var(--color-foreground)] mb-2">Smart retries</h3>
						<p class="text-sm text-[var(--color-muted-foreground)]">
							Exponential backoff from 1s to 1h. Failed webhooks move to dead-letter after 7 days. Never lose a webhook.
						</p>
					</div>

					<div class="p-6 rounded-xl border border-[var(--color-border)] bg-[var(--color-background)]">
						<div class="w-10 h-10 rounded-lg bg-purple-500/10 flex items-center justify-center text-purple-500 mb-4">
							<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
							</svg>
						</div>
						<h3 class="text-lg font-semibold text-[var(--color-foreground)] mb-2">Request history</h3>
						<p class="text-sm text-[var(--color-muted-foreground)]">
							Full webhook history with headers, body, and delivery attempts. Replay any webhook with one click.
						</p>
					</div>

					<div class="p-6 rounded-xl border border-[var(--color-border)] bg-[var(--color-background)]">
						<div class="w-10 h-10 rounded-lg bg-orange-500/10 flex items-center justify-center text-orange-500 mb-4">
							<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
							</svg>
						</div>
						<h3 class="text-lg font-semibold text-[var(--color-foreground)] mb-2">Simple CLI</h3>
						<p class="text-sm text-[var(--color-muted-foreground)]">
							One binary, zero config. <code class="px-1 py-0.5 rounded bg-[var(--color-muted)] text-xs">go install hooks.dx314.com/hookly@latest</code> and you're ready.
						</p>
					</div>

					<div class="p-6 rounded-xl border border-[var(--color-border)] bg-[var(--color-background)]">
						<div class="w-10 h-10 rounded-lg bg-red-500/10 flex items-center justify-center text-red-500 mb-4">
							<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
							</svg>
						</div>
						<h3 class="text-lg font-semibold text-[var(--color-foreground)] mb-2">Encrypted at rest</h3>
						<p class="text-sm text-[var(--color-muted-foreground)]">
							Webhook secrets and payloads encrypted with AES-256-GCM. Your data is secure on our servers.
						</p>
					</div>

					<div class="p-6 rounded-xl border border-[var(--color-border)] bg-[var(--color-background)]">
						<div class="w-10 h-10 rounded-lg bg-cyan-500/10 flex items-center justify-center text-cyan-500 mb-4">
							<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
							</svg>
						</div>
						<h3 class="text-lg font-semibold text-[var(--color-foreground)] mb-2">Real-time streaming</h3>
						<p class="text-sm text-[var(--color-muted-foreground)]">
							Webhooks arrive instantly via gRPC streaming. No polling. No delays. Sub-second delivery to your machine.
						</p>
					</div>
				</div>
			</div>
		</div>

		<!-- Self-hosting -->
		<div class="py-24 bg-[var(--color-muted)]/30">
			<div class="max-w-screen-xl mx-auto px-4">
				<div class="grid md:grid-cols-2 gap-12 items-center">
					<div>
						<div class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-[var(--color-background)] border border-[var(--color-border)] text-sm text-[var(--color-muted-foreground)] mb-6">
							<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
							</svg>
							Open Source
						</div>
						<h2 class="text-3xl font-bold text-[var(--color-foreground)]">Self-host on your own infrastructure</h2>
						<p class="mt-4 text-lg text-[var(--color-muted-foreground)]">
							Run the entire Hookly stack on your own servers. Full control over your data, no external dependencies.
						</p>
						<ul class="mt-6 space-y-3">
							<li class="flex items-start gap-3">
								<svg class="w-5 h-5 text-green-500 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
								</svg>
								<span class="text-[var(--color-muted-foreground)]">Single binary deployment with SQLite storage</span>
							</li>
							<li class="flex items-start gap-3">
								<svg class="w-5 h-5 text-green-500 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
								</svg>
								<span class="text-[var(--color-muted-foreground)]">Docker and Docker Compose support included</span>
							</li>
							<li class="flex items-start gap-3">
								<svg class="w-5 h-5 text-green-500 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
								</svg>
								<span class="text-[var(--color-muted-foreground)]">GitHub OAuth or bring your own auth</span>
							</li>
							<li class="flex items-start gap-3">
								<svg class="w-5 h-5 text-green-500 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
								</svg>
								<span class="text-[var(--color-muted-foreground)]">MIT licensed, fork and customize freely</span>
							</li>
						</ul>
						<div class="mt-8">
							<a
								href="https://github.com/dx314/hookly"
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex items-center gap-2 text-[var(--color-foreground)] font-medium hover:underline"
							>
								View on GitHub
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14 5l7 7m0 0l-7 7m7-7H3" />
								</svg>
							</a>
						</div>
					</div>
					<div class="rounded-xl border border-[var(--color-border)] bg-[#0a0a0a] overflow-hidden">
						<div class="flex items-center justify-between px-4 py-3 bg-[#1a1a1a] border-b border-[#333]">
							<div class="flex items-center gap-2">
								<span class="h-3 w-3 rounded-full bg-[#ff5f56]"></span>
								<span class="h-3 w-3 rounded-full bg-[#ffbd2e]"></span>
								<span class="h-3 w-3 rounded-full bg-[#27ca40]"></span>
							</div>
							<span class="text-xs text-[#888]">docker-compose.yml</span>
						</div>
						<pre class="p-6 font-mono text-sm text-[#e5e5e5] overflow-x-auto"><code class="text-[#888]"># Clone and run</code>
git clone https://github.com/dx314/hookly
cd hookly

<code class="text-[#888]"># Configure environment</code>
cp .env.example .env
vim .env

<code class="text-[#888]"># Start the edge gateway</code>
docker compose up -d

<code class="text-[#888]"># Your instance is now live at</code>
<code class="text-green-400"># https://your-domain.com</code></pre>
					</div>
				</div>
			</div>
		</div>

		<!-- Quick start code -->
		<div class="py-24">
			<div class="max-w-screen-xl mx-auto px-4">
				<div class="max-w-2xl mx-auto">
					<div class="text-center mb-12">
						<h2 class="text-3xl font-bold text-[var(--color-foreground)]">Get started in seconds</h2>
						<p class="mt-4 text-lg text-[var(--color-muted-foreground)]">Install, login, and start receiving webhooks</p>
					</div>

					<div class="rounded-xl border border-[var(--color-border)] bg-[#0a0a0a] overflow-hidden">
						<div class="flex items-center justify-between px-4 py-3 bg-[#1a1a1a] border-b border-[#333]">
							<div class="flex items-center gap-2">
								<span class="h-3 w-3 rounded-full bg-[#ff5f56]"></span>
								<span class="h-3 w-3 rounded-full bg-[#ffbd2e]"></span>
								<span class="h-3 w-3 rounded-full bg-[#27ca40]"></span>
							</div>
							<span class="text-xs text-[#888]">bash</span>
						</div>
						<div class="p-6 font-mono text-sm space-y-4">
							<div>
								<div class="text-[#888]"># Install the CLI</div>
								<div class="text-[#e5e5e5]">go install hooks.dx314.com/hookly@latest</div>
							</div>
							<div>
								<div class="text-[#888]"># Authenticate with GitHub</div>
								<div class="text-[#e5e5e5]">hookly login</div>
							</div>
							<div>
								<div class="text-[#888]"># Start relaying webhooks to localhost:3000</div>
								<div class="text-[#e5e5e5]">hookly</div>
							</div>
						</div>
					</div>

					<div class="mt-8 text-center">
						<a
							href="/auth/login"
							class="inline-flex items-center justify-center gap-2 rounded-lg bg-[var(--color-foreground)] px-6 py-3 text-base font-medium text-[var(--color-background)] hover:opacity-90 transition-opacity"
						>
							<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
								<path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
							</svg>
							Start for free
						</a>
						<p class="mt-4 text-sm text-[var(--color-muted-foreground)]">
							No credit card required. Free for personal use.
						</p>
					</div>
				</div>
			</div>
		</div>

		<!-- Footer -->
		<footer class="py-12 border-t border-[var(--color-border)]">
			<div class="max-w-screen-xl mx-auto px-4">
				<div class="flex flex-col md:flex-row items-center justify-between gap-4">
					<div class="flex items-center gap-2">
						<span class="text-xl">🪝</span>
						<span class="font-semibold text-[var(--color-foreground)]">Hookly</span>
					</div>
					<p class="text-sm text-[var(--color-muted-foreground)]">
						Webhook relay for local development
					</p>
					<div class="flex items-center gap-6">
						<a href="/cli" class="text-sm text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)] transition-colors">
							Documentation
						</a>
						<a href="https://github.com/dx314/hookly" class="text-sm text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)] transition-colors">
							GitHub
						</a>
					</div>
				</div>
			</div>
		</footer>
	</div>
{:else}
	<!-- Dashboard for logged-in users -->
	<div class="space-y-6">
		<div>
			<h1 class="text-2xl font-bold text-[var(--color-foreground)]">Dashboard</h1>
			<p class="text-[var(--color-muted-foreground)]">Monitor your webhook relay system</p>
		</div>

		{#if error}
			<div class="rounded-lg border border-[var(--color-destructive)] bg-[var(--color-destructive)]/10 p-4">
				<p class="text-[var(--color-destructive)]">{error}</p>
			</div>
		{:else if status}
			<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
				<!-- Connected Relays -->
				<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6">
					<div class="flex items-center justify-between">
						<p class="text-sm font-medium text-[var(--color-muted-foreground)]">Relays</p>
						<span class="flex h-3 w-3 rounded-full {status.connectedEndpoints.length > 0 ? 'bg-green-500' : 'bg-zinc-500'}"></span>
					</div>
					<p class="text-2xl font-bold text-[var(--color-foreground)] mt-2">
						{status.connectedEndpoints.length} online
					</p>
					{#if status.connectedEndpoints.length > 0}
						<div class="mt-2 flex flex-wrap gap-1">
							{#each status.connectedEndpoints as endpoint (endpoint.id)}
								<a
									href="/endpoints/{endpoint.id}"
									class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-500/10 text-green-600 hover:bg-green-500/20 transition-colors"
								>
									{endpoint.name}
								</a>
							{/each}
						</div>
					{:else}
						<p class="text-xs text-[var(--color-muted-foreground)] mt-1">no endpoints connected</p>
					{/if}
				</div>

				<!-- Pending Webhooks -->
				<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6">
					<p class="text-sm font-medium text-[var(--color-muted-foreground)]">Pending</p>
					<p class="text-2xl font-bold text-[var(--color-status-pending)] mt-2">{status.pendingCount}</p>
					<p class="text-xs text-[var(--color-muted-foreground)] mt-1">webhooks awaiting delivery</p>
				</div>

				<!-- Failed Webhooks -->
				<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6">
					<p class="text-sm font-medium text-[var(--color-muted-foreground)]">Failed</p>
					<p class="text-2xl font-bold text-[var(--color-status-failed)] mt-2">{status.failedCount}</p>
					<p class="text-xs text-[var(--color-muted-foreground)] mt-1">permanent failures</p>
				</div>

				<!-- Dead Letter -->
				<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6">
					<p class="text-sm font-medium text-[var(--color-muted-foreground)]">Dead Letter</p>
					<p class="text-2xl font-bold text-[var(--color-status-dead-letter)] mt-2">{status.deadLetterCount}</p>
					<p class="text-xs text-[var(--color-muted-foreground)] mt-1">exceeded retry window</p>
				</div>
			</div>
		{/if}

		<!-- Quick Actions -->
		<div class="grid grid-cols-1 md:grid-cols-2 gap-4 mt-8">
			<a
				href="/endpoints/new"
				class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6 hover:border-[var(--color-foreground)] transition-colors"
			>
				<h3 class="text-lg font-semibold text-[var(--color-foreground)]">Create Endpoint</h3>
				<p class="text-sm text-[var(--color-muted-foreground)] mt-1">
					Set up a new webhook endpoint for Stripe, GitHub, or other providers
				</p>
			</a>
			<a
				href="/webhooks"
				class="rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] p-6 hover:border-[var(--color-foreground)] transition-colors"
			>
				<h3 class="text-lg font-semibold text-[var(--color-foreground)]">View Webhooks</h3>
				<p class="text-sm text-[var(--color-muted-foreground)] mt-1">
					Browse and manage received webhooks, replay failed deliveries
				</p>
			</a>
		</div>
	</div>
{/if}
