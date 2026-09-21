<script lang="ts">
	import { onMount } from 'svelte';

	// Everything arrives in the URL fragment so it never reaches a server or its logs.
	let code = $state('');
	let username = $state('');
	let callback = $state<'trying' | 'connected' | 'unreachable' | 'none'>('none');
	let callbackURL = $state('');
	let copied = $state(false);
	let invalid = $state(false);

	function decodeCode(value: string): { token: string; user_id: string; username: string; state: string } | null {
		try {
			const b64 = value.replace(/^hkc_/, '').replace(/-/g, '+').replace(/_/g, '/');
			const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
			return JSON.parse(new TextDecoder().decode(bytes));
		} catch {
			return null;
		}
	}

	onMount(() => {
		const params = new URLSearchParams(window.location.hash.slice(1));
		code = params.get('code') ?? '';
		const port = params.get('port') ?? '';
		const payload = decodeCode(code);

		// Don't leave the token in the address bar or history
		history.replaceState(null, '', window.location.pathname);

		if (!payload?.token) {
			invalid = true;
			return;
		}
		username = payload.username;

		if (!/^\d+$/.test(port)) return;

		const query = new URLSearchParams({
			token: payload.token,
			state: payload.state,
			user_id: payload.user_id,
			username: payload.username
		});
		callbackURL = `http://localhost:${port}/callback?${query}`;

		// Works when this browser runs on the same machine as the CLI. If the
		// CLI is on another machine (SSH), it fails and the code below is used.
		callback = 'trying';
		fetch(`${callbackURL}&via=fetch`, { mode: 'cors' })
			.then((res) => {
				callback = res.ok ? 'connected' : 'unreachable';
			})
			.catch(() => {
				callback = 'unreachable';
			});
	});

	async function copyCode() {
		try {
			await navigator.clipboard.writeText(code);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			// Clipboard unavailable - the code is selectable
		}
	}
</script>

<div class="flex min-h-[60vh] items-center justify-center">
	<div class="w-full max-w-md space-y-6">
		{#if invalid}
			<div class="rounded-lg border border-[var(--color-destructive)] bg-[var(--color-destructive)]/10 p-4">
				<p class="text-[var(--color-destructive)]">No login code found. Run <code>hookly login</code> again and use the URL it prints.</p>
			</div>
		{:else if callback === 'connected'}
			<div class="text-center space-y-4">
				<div class="flex items-center justify-center w-14 h-14 mx-auto rounded-full bg-green-500/10 border border-green-500/20">
					<svg class="w-7 h-7 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
					</svg>
				</div>
				<h1 class="text-xl font-semibold text-[var(--color-foreground)]">You're all set!</h1>
				<p class="text-sm text-[var(--color-muted-foreground)]">
					Logged in as <span class="font-medium text-[var(--color-foreground)]">{username}</span>. Your CLI is now
					connected to Hookly.<br />You can close this window.
				</p>
			</div>
		{:else if code}
			<div class="text-center space-y-2">
				<h1 class="text-xl font-semibold text-[var(--color-foreground)]">Paste this code into your terminal</h1>
				<p class="text-sm text-[var(--color-muted-foreground)]">
					Signed in as <span class="font-medium text-[var(--color-foreground)]">{username}</span>.
					{#if callback === 'trying'}
						Trying to reach the CLI on this machine&hellip;
					{:else if callback === 'unreachable'}
						The CLI isn't running on this machine, so finish the login by pasting the code where
						<code>hookly login</code> is waiting.
					{:else}
						Paste it where <code>hookly login</code> is waiting and press Enter.
					{/if}
				</p>
			</div>

			<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-muted)] p-4 space-y-3">
				<pre class="whitespace-pre-wrap break-all text-xs font-mono text-[var(--color-foreground)] select-all">{code}</pre>
				<button
					type="button"
					onclick={copyCode}
					class="w-full rounded-lg bg-[var(--color-foreground)] px-4 py-2.5 text-sm font-medium text-[var(--color-background)] hover:opacity-90 transition-opacity"
				>
					{copied ? 'Copied' : 'Copy code'}
				</button>
			</div>

			<p class="text-xs text-center text-[var(--color-muted-foreground)]">
				Treat this code like a password: it gives full API access to your account. The CLI only accepts it for the
				login attempt that produced it.
				{#if callbackURL && callback === 'unreachable'}
					<br />CLI on this machine after all? <a class="underline" href={callbackURL}>Retry the automatic login</a>.
				{/if}
			</p>
		{/if}
	</div>
</div>
