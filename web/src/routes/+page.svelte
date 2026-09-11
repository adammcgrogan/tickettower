<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { getMe, loginURL, type TicketType, type User } from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import Icon from '$lib/components/Icon.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import PanelPreview from '$lib/components/PanelPreview.svelte';

	const GITHUB_URL = 'https://github.com/adammcgrogan/tickettower';

	let user = $state<User | null>(null);
	const loginFailed = $derived(page.url.searchParams.get('error') === 'login_failed');

	onMount(async () => {
		user = await getMe().catch(() => null);
	});

	const features = [
		{
			title: 'Live in minutes',
			body: 'Answer a few questions and your ticket buttons are posted, as buttons or a dropdown menu.'
		},
		{
			title: 'Channels or private threads',
			body: 'Each kind of ticket opens where it suits your server, visible only to the member and your team.'
		},
		{
			title: 'Questions up front',
			body: 'Ask for an order number or a description before the ticket opens, so nobody starts from scratch.'
		},
		{
			title: 'Knows whose turn it is',
			body: 'See which tickets are waiting on your team. Quiet ones get a reminder, then close themselves.'
		},
		{
			title: 'Transcripts',
			body: 'Every conversation is saved and readable in the dashboard, laid out just like Discord.'
		},
		{
			title: 'Ratings and response times',
			body: 'Members rate their ticket when it closes, and you see how quickly your team replies.'
		}
	];

	const demoTypes = [
		{ id: 1, name: 'General support', emoji: '💬' },
		{ id: 2, name: 'Billing', emoji: '💳' },
		{ id: 3, name: 'Report a member', emoji: '🚩' }
	] as TicketType[];
</script>

<svelte:head><title>{APP_NAME} · Ticket bot for Discord</title></svelte:head>

<div class="min-h-dvh">
	<header class="mx-auto flex h-16 max-w-6xl items-center justify-between px-5">
		<Logo />
		<nav class="flex items-center gap-1 text-sm">
			<a href={GITHUB_URL} target="_blank" rel="noopener" class="btn btn-ghost">GitHub</a>
			{#if user}
				<a href="/servers" class="btn btn-secondary">Dashboard</a>
			{:else}
				<a href={loginURL} data-sveltekit-reload class="btn btn-secondary">Log in</a>
			{/if}
		</nav>
	</header>

	<main>
		{#if loginFailed}
			<div class="mx-auto mt-4 max-w-md px-5">
				<div class="rounded-lg border border-danger/30 bg-danger/10 px-4 py-2.5 text-center text-sm text-danger">
					Login didn't complete. Please try again.
				</div>
			</div>
		{/if}

		<section class="mx-auto grid max-w-6xl items-center gap-14 px-5 pt-14 pb-24 lg:grid-cols-[1.15fr_1fr] lg:pt-24">
			<div>
				<h1 class="font-display text-6xl leading-[0.92] font-extrabold text-balance sm:text-7xl lg:text-8xl">
					Support tickets for Discord, without the clutter.
				</h1>
				<p class="mt-7 max-w-lg text-lg text-pretty text-muted">
					{APP_NAME} opens a private channel or thread for every request, shows your team who is waiting
					for a reply, and keeps a transcript when it's done. Free and open source.
				</p>
				<div class="mt-9 flex flex-col gap-3 sm:flex-row">
					<a href="/api/invite" data-sveltekit-reload class="btn btn-primary h-11 px-5">
						Add to Discord <Icon name="arrow-right" size={15} />
					</a>
					<a
						href={user ? '/servers' : loginURL}
						data-sveltekit-reload={user ? undefined : true}
						class="btn btn-secondary h-11 px-5"
					>
						Open the dashboard
					</a>
				</div>
			</div>

			<div class="rounded-2xl border border-border bg-surface p-2" aria-label="Example ticket buttons">
				<PanelPreview
					title="Need a hand?"
					description="Pick a topic below and we'll open a private ticket for you. Our team will be with you shortly."
					color={0xf2b544}
					style="buttons"
					types={demoTypes}
				/>
			</div>
		</section>

		<section class="border-t border-border">
			<div class="mx-auto max-w-6xl px-5 py-20">
				<h2 class="font-display text-4xl font-bold">What it does</h2>
				<dl class="mt-10 grid gap-x-12 gap-y-10 sm:grid-cols-2 lg:grid-cols-3">
					{#each features as f (f.title)}
						<div class="border-t border-border pt-5">
							<dt class="font-semibold">{f.title}</dt>
							<dd class="mt-2 text-sm leading-relaxed text-muted">{f.body}</dd>
						</div>
					{/each}
				</dl>
			</div>
		</section>
	</main>

	<footer class="border-t border-border">
		<div class="mx-auto flex max-w-6xl items-center justify-between px-5 py-6 text-xs text-subtle">
			<span>© {new Date().getFullYear()} {APP_NAME}</span>
			<div class="flex gap-5">
				<a href="/privacy" class="transition-colors hover:text-fg">Privacy</a>
				<a href={GITHUB_URL} target="_blank" rel="noopener" class="transition-colors hover:text-fg">
					Open source on GitHub
				</a>
			</div>
		</div>
	</footer>
</div>
