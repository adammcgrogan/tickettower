<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { getMe, loginURL, type User } from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import Icon from '$lib/components/Icon.svelte';
	import Logo from '$lib/components/Logo.svelte';

	const GITHUB_URL = 'https://github.com/adammcgrogan/ticketsbot';

	let user = $state<User | null>(null);
	const loginFailed = $derived(page.url.searchParams.get('error') === 'login_failed');

	onMount(async () => {
		user = await getMe().catch(() => null);
	});

	const features = [
		{
			icon: 'panel',
			title: 'Panels in minutes',
			body: 'Build ticket panels with buttons or dropdowns and see a live preview before you publish.'
		},
		{
			icon: 'tag',
			title: 'Channels or threads',
			body: 'Each ticket type can open a private channel or a private thread. Pick what suits your server.'
		},
		{
			icon: 'transcript',
			title: 'Transcripts',
			body: 'Every closed ticket is saved and readable in the dashboard, laid out just like Discord.'
		},
		{
			icon: 'chart',
			title: 'Analytics & feedback',
			body: 'Response times, volume and ratings from members, so you know how your team is doing.'
		}
	] as const;

	const panelButtons = ['General support', 'Billing', 'Report a user'];
</script>

<svelte:head><title>{APP_NAME} · Ticket bot for Discord</title></svelte:head>

<div class="relative min-h-dvh overflow-hidden">
	<!-- Background: faint grid with an accent glow behind the hero. -->
	<div
		aria-hidden="true"
		class="pointer-events-none absolute inset-0 [mask-image:radial-gradient(ellipse_at_top,black_20%,transparent_70%)]"
		style="background-image:linear-gradient(var(--color-border) 1px,transparent 1px),linear-gradient(90deg,var(--color-border) 1px,transparent 1px);background-size:56px 56px;opacity:.35"
	></div>
	<div
		aria-hidden="true"
		class="pointer-events-none absolute top-[-280px] left-1/2 h-[560px] w-[900px] -translate-x-1/2 rounded-full bg-accent/20 blur-[120px]"
	></div>

	<header class="relative z-10 mx-auto flex h-16 max-w-6xl items-center justify-between px-5">
		<Logo />
		<nav class="flex items-center gap-1 text-sm">
			<a
				href={GITHUB_URL}
				target="_blank"
				rel="noopener"
				class="rounded-lg px-3 py-2 text-muted transition-colors hover:text-fg">GitHub</a
			>
			{#if user}
				<a
					href="/servers"
					class="rounded-lg border border-border bg-surface px-3 py-2 transition-colors hover:bg-elevated"
					>Dashboard</a
				>
			{:else}
				<a
					href={loginURL}
					data-sveltekit-reload
					class="rounded-lg border border-border bg-surface px-3 py-2 transition-colors hover:bg-elevated"
					>Log in</a
				>
			{/if}
		</nav>
	</header>

	<main class="relative z-10">
		{#if loginFailed}
			<div class="mx-auto mt-4 max-w-md px-5">
				<div class="rounded-lg border border-danger/30 bg-danger/10 px-4 py-2.5 text-center text-sm text-danger">
					Login didn't complete. Please try again.
				</div>
			</div>
		{/if}

		<section class="mx-auto max-w-3xl px-5 pt-20 pb-16 text-center sm:pt-28">
			<span
				class="inline-flex items-center gap-2 rounded-full border border-border bg-surface/80 px-3 py-1 text-xs text-muted"
			>
				<span class="size-1.5 rounded-full bg-success"></span>
				Free & open source
			</span>
			<h1 class="mt-6 text-4xl font-semibold tracking-tight text-balance sm:text-6xl">
				Support tickets,<br />without the clutter.
			</h1>
			<p class="mx-auto mt-5 max-w-xl text-base text-pretty text-muted sm:text-lg">
				{APP_NAME} is a ticket bot for Discord that you can set up in minutes, from a dashboard that's
				easy to use.
			</p>
			<div class="mt-9 flex flex-col items-center justify-center gap-3 sm:flex-row">
				<a
					href="/api/invite"
					data-sveltekit-reload
					class="inline-flex h-11 items-center gap-2 rounded-lg bg-accent px-5 text-sm font-medium text-white shadow-lg shadow-accent/25 transition-colors hover:bg-accent-hover"
				>
					Add to Discord
					<Icon name="arrow-right" size={15} />
				</a>
				<a
					href={user ? '/servers' : loginURL}
					data-sveltekit-reload={user ? undefined : true}
					class="inline-flex h-11 items-center rounded-lg border border-border bg-surface px-5 text-sm font-medium transition-colors hover:bg-elevated"
				>
					Open dashboard
				</a>
			</div>
		</section>

		<!-- A mock of a ticket panel as it appears in Discord. -->
		<section class="mx-auto max-w-2xl px-5" aria-label="Example ticket panel">
			<div class="rounded-2xl border border-border bg-surface/80 p-2 shadow-2xl shadow-black/60 backdrop-blur">
				<div class="rounded-xl bg-[#313338] p-5 text-left">
					<div class="flex gap-4">
						<div class="grid size-10 shrink-0 place-items-center rounded-full bg-accent">
							<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="white" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 12h10" /><path d="m10 6 6 6-6 6" /><path d="M20 5v14" /></svg>
						</div>
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<span class="text-[15px] font-medium text-white">{APP_NAME}</span>
								<span class="rounded bg-[#5865f2] px-1.5 py-px text-[10px] font-semibold text-white">APP</span>
								<span class="text-xs text-[#949ba4]">Today at 09:41</span>
							</div>
							<div class="mt-2 rounded border-l-4 border-accent bg-[#2b2d31] p-4">
								<div class="font-semibold text-white">Need a hand?</div>
								<p class="mt-1 text-sm text-[#dbdee1]">
									Pick a topic below and our team will be with you shortly. Tickets are private
									between you and staff.
								</p>
							</div>
							<div class="mt-2 flex flex-wrap gap-2">
								{#each panelButtons as label, i (label)}
									<span
										class="rounded px-4 py-1.5 text-sm font-medium text-white {i === 0
											? 'bg-[#5865f2]'
											: 'bg-[#4e5058]'}">{label}</span
									>
								{/each}
							</div>
						</div>
					</div>
				</div>
			</div>
		</section>

		<section class="mx-auto grid max-w-6xl gap-3 px-5 py-24 sm:grid-cols-2 lg:grid-cols-4">
			{#each features as f (f.title)}
				<div class="rounded-xl border border-border bg-surface/60 p-5">
					<div class="grid size-9 place-items-center rounded-lg border border-border bg-elevated text-accent">
						<Icon name={f.icon} />
					</div>
					<h3 class="mt-4 font-medium">{f.title}</h3>
					<p class="mt-1.5 text-sm leading-relaxed text-muted">{f.body}</p>
				</div>
			{/each}
		</section>
	</main>

	<footer class="relative z-10 border-t border-border">
		<div class="mx-auto flex max-w-6xl items-center justify-between px-5 py-6 text-xs text-subtle">
			<span>© {new Date().getFullYear()} {APP_NAME}</span>
			<div class="flex gap-5">
				<a href="/privacy" class="transition-colors hover:text-fg">Privacy</a>
				<a href={GITHUB_URL} target="_blank" rel="noopener" class="transition-colors hover:text-fg"
					>Open source on GitHub</a
				>
			</div>
		</div>
	</footer>
</div>
