<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { getMe, loginURL, type TicketType, type User } from '$lib/api';
	import { APP_NAME, SUPPORT_URL } from '$lib/brand';
	import Icon from '$lib/components/Icon.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import PanelPreview from '$lib/components/PanelPreview.svelte';
	import TicketStub from '$lib/components/TicketStub.svelte';
	import QueueDemo from '$lib/components/landing/QueueDemo.svelte';
	import Walkthrough from '$lib/components/landing/Walkthrough.svelte';

	const GITHUB_URL = 'https://github.com/adammcgrogan/tickettower';

	let user = $state<User | null>(null);
	const loginFailed = $derived(page.url.searchParams.get('error') === 'login_failed');

	onMount(async () => {
		user = await getMe().catch(() => null);
	});

	const promises = ['Free to use', 'Open source', 'No code or config files'];

	const steps = [
		{
			title: 'Add the bot',
			body: 'Invite it to your server. It asks only for the permissions tickets need.'
		},
		{
			title: 'Answer a few questions',
			body: 'Quick setup asks what tickets are for and who handles them, then creates your first ticket type.'
		},
		{
			title: 'Post your ticket buttons',
			body: 'Pick a channel and publish. Members can open tickets straight away.'
		}
	];

	const features = [
		{
			icon: 'panel' as const,
			title: 'Buttons or a dropdown',
			body: 'With a live preview of how it looks in Discord.'
		},
		{
			icon: 'thread' as const,
			title: 'Channels or private threads',
			body: 'Pick per ticket type, visible only to the member and your team.'
		},
		{
			icon: 'message' as const,
			title: 'Questions up front',
			body: 'So nobody starts from scratch.'
		},
		{
			icon: 'inbox' as const,
			title: 'Knows whose turn it is',
			body: 'The longest-waiting member rises to the top of your queue.'
		},
		{
			icon: 'send' as const,
			title: 'Reply from the dashboard',
			body: 'Posted in Discord under your name and avatar.'
		},
		{
			icon: 'clock' as const,
			title: 'Quiet tickets close themselves',
			body: 'A friendly reminder first, then a close if nobody replies.'
		},
		{
			icon: 'transcript' as const,
			title: 'Transcripts',
			body: 'Saved and readable in the dashboard, laid out like Discord.'
		},
		{
			icon: 'chart' as const,
			title: 'Ratings and response times',
			body: 'Response times, busiest hours and each person on your team.'
		},
		{
			icon: 'alert' as const,
			title: 'Finds problems before members do',
			body: 'A setup check links straight to the fix.'
		}
	];

	const comparison: { label: string; tower: string | boolean; a: string | boolean; b: string | boolean }[] = [
		{ label: 'Starting price', tower: 'Free', a: 'Free, Pro from $8/mo', b: 'Free, Premium from $2.99/mo' },
		{ label: 'Reply to tickets from a browser', tower: true, a: false, b: true },
		{ label: 'Analytics and response times', tower: true, a: false, b: false },
		{ label: 'Quiet tickets close themselves', tower: true, a: true, b: false }
	];

	const faqs: { q: string; a: string; link?: { href: string; label: string } }[] = [
		{
			q: 'Is it really free?',
			a: 'The free plan covers everything most servers need: transcripts, analytics and replying from the dashboard, with generous limits on ticket types and ticket buttons. A paid premium plan raises those limits for larger servers.'
		},
		{
			q: 'How long does setup take?',
			a: 'A few minutes. Add the bot, answer the quick setup questions on the Home page and post your ticket buttons. The setup check tells you if a permission is missing.',
			link: { href: '/help/getting-started', label: 'Read the setup guide' }
		},
		{
			q: 'Should tickets open as channels or threads?',
			a: 'Channels suit busy servers that want tickets in their own category. Private threads keep your channel list tidy. You can pick per ticket type.',
			link: { href: '/help/channels-vs-threads', label: 'Compare them' }
		},
		{
			q: 'Who can see a ticket?',
			a: "The member who opened it, the support roles you choose for that ticket type, anyone your team adds, and your server's admins. Nobody else."
		},
		{
			q: 'What does it store?',
			a: 'Messages sent in ticket channels and threads, so it can show transcripts. Nothing from the rest of your server. You choose how long transcripts are kept.',
			link: { href: '/privacy', label: 'Read the privacy page' }
		},
		{
			q: 'How many ticket types can I have?',
			a: 'Up to 25 ticket types and 10 sets of ticket buttons per server. Each set holds up to 25 buttons, which is as many as Discord allows in one message.'
		},
		{
			q: "I'm using another ticket bot. Can I switch?",
			a: 'Add it alongside your current bot and post new ticket buttons. Once the old tickets are finished, remove the old buttons and bot.'
		},
		{
			q: 'Can I host it myself?',
			a: `${APP_NAME} is open source under the MIT licence. The README explains how to run your own copy.`,
			link: { href: GITHUB_URL, label: 'View on GitHub' }
		}
	];

	const demoTypes = [
		{ id: 1, name: 'General support', emoji: '💬' },
		{ id: 2, name: 'Billing', emoji: '💳' },
		{ id: 3, name: 'Report a member', emoji: '🚩' }
	] as TicketType[];

	const external = (href: string) => href.startsWith('http');
</script>

<svelte:head>
	<title>{APP_NAME} · Ticket bot for Discord</title>
	<meta
		name="description"
		content="{APP_NAME} is a free, open source ticket bot for Discord. Members open private tickets with one click, your team sees who is waiting, and every conversation is saved."
	/>
</svelte:head>

{#snippet ctas(size = 'h-11 px-5')}
	<div class="flex flex-col gap-3 sm:flex-row">
		<a href="/api/invite" data-sveltekit-reload class="btn btn-primary {size}">
			Add to Discord <Icon name="arrow-right" size={15} />
		</a>
		<a
			href={user ? '/servers' : loginURL}
			data-sveltekit-reload={user ? undefined : true}
			class="btn btn-secondary {size}"
		>
			Open the dashboard
		</a>
	</div>
{/snippet}

<div class="min-h-dvh">
	<header class="sticky top-0 z-20 border-b border-transparent bg-bg/80 backdrop-blur-md">
		<div class="mx-auto flex h-16 max-w-6xl items-center justify-between px-5">
			<a href="/" aria-label="{APP_NAME} home"><Logo /></a>
			<nav class="flex items-center gap-1 text-sm">
				<a href="#how-it-works" class="btn btn-ghost hidden md:inline-flex">How it works</a>
				<a href="#features" class="btn btn-ghost hidden md:inline-flex">Features</a>
				<a href="#faq" class="btn btn-ghost hidden md:inline-flex">FAQ</a>
				<a href="/help" class="btn btn-ghost">Help</a>
				{#if user}
					<a href="/servers" class="btn btn-secondary">Dashboard</a>
				{:else}
					<a href={loginURL} data-sveltekit-reload class="btn btn-secondary">Log in</a>
				{/if}
			</nav>
		</div>
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
					for a reply, and keeps a transcript when it's done.
				</p>
				<div class="mt-9">{@render ctas()}</div>
				<ul class="mt-7 flex flex-wrap gap-x-6 gap-y-2 text-sm text-muted">
					{#each promises as p (p)}
						<li class="flex items-center gap-2"><Icon name="check" size={15} class="text-success" />{p}</li>
					{/each}
				</ul>
			</div>

			<div class="rounded-2xl border border-border bg-surface p-2" aria-label="Example ticket buttons">
				<PanelPreview
					title="Need a hand?"
					description="Pick a topic below and we'll open a private ticket for you. Our team will be with you shortly."
					color={0xf2b544}
					style="buttons"
					types={demoTypes}
				/>
				<div class="flex items-center gap-3 px-3 pt-3.5 pb-2" aria-label="The ticket that opens">
					<TicketStub number={42} tone="waiting" />
					<div class="min-w-0 flex-1">
						<div class="truncate text-sm font-medium">Billing, opened by alex</div>
						<div class="truncate text-xs text-accent">Waiting on your team</div>
					</div>
					<span class="hidden text-xs text-subtle sm:block">Just now</span>
				</div>
			</div>
		</section>

		<section id="how-it-works" class="scroll-mt-16 border-t border-border">
			<div class="mx-auto max-w-6xl px-5 py-20">
				<h2 class="font-display text-4xl font-bold">Live in a few minutes</h2>
				<p class="mt-3 max-w-xl text-muted">
					No config files, no commands to memorise. Everything is set up in the dashboard.
				</p>
				<ol class="mt-10 grid gap-x-12 gap-y-10 md:grid-cols-3">
					{#each steps as s, i (s.title)}
						<li class="border-t border-border pt-5">
							<TicketStub number={i + 1} size="sm" />
							<h3 class="mt-4 font-semibold">{s.title}</h3>
							<p class="mt-2 text-sm leading-relaxed text-muted">{s.body}</p>
						</li>
					{/each}
				</ol>
			</div>
		</section>

		<section class="border-t border-border">
			<div class="mx-auto max-w-6xl px-5 py-20">
				<h2 class="font-display text-4xl font-bold">A ticket, start to finish</h2>
				<p class="mt-3 max-w-xl text-muted">
					This is what your members and your team see in Discord. Pick a step to follow one along.
				</p>
				<div class="mt-10"><Walkthrough /></div>
			</div>
		</section>

		<section class="border-t border-border">
			<div class="mx-auto grid max-w-6xl items-center gap-12 px-5 py-20 lg:grid-cols-[1fr_1.6fr]">
				<div>
					<h2 class="font-display text-4xl font-bold text-balance">Never lose track of who's waiting</h2>
					<p class="mt-4 text-muted">
						The dashboard knows whose turn it is. Tickets waiting on your team are marked in amber, longest wait
						first, so nothing slips through while everyone assumes someone else has it.
					</p>
					<ul class="mt-6 space-y-3 text-sm">
						<li class="flex gap-3">
							<Icon name="inbox" size={16} class="mt-0.5 text-muted" />
							<span>Search every ticket by number, member, type or who claimed it.</span>
						</li>
						<li class="flex gap-3">
							<Icon name="send" size={16} class="mt-0.5 text-muted" />
							<span>Read the conversation and reply without opening Discord.</span>
						</li>
						<li class="flex gap-3">
							<Icon name="chart" size={16} class="mt-0.5 text-muted" />
							<span>See response times, busiest hours and ratings, compared with last period.</span>
						</li>
					</ul>
				</div>
				<QueueDemo />
			</div>
		</section>

		<section id="features" class="scroll-mt-16 border-t border-border">
			<div class="mx-auto max-w-6xl px-5 py-20">
				<h2 class="font-display text-4xl font-bold">Everything you need, nothing you don't</h2>
				<dl class="mt-10 grid gap-x-10 gap-y-8 sm:grid-cols-2 lg:grid-cols-3">
					{#each features as f (f.title)}
						<div class="flex gap-3.5">
							<Icon name={f.icon} size={18} class="mt-0.5 shrink-0 text-accent" />
							<div>
								<dt class="font-semibold">{f.title}</dt>
								<dd class="mt-1 text-sm leading-relaxed text-muted">{f.body}</dd>
							</div>
						</div>
					{/each}
				</dl>
			</div>
		</section>

		<section class="border-t border-border">
			<div class="mx-auto max-w-6xl px-5 py-20">
				<h2 class="font-display text-4xl font-bold">Open, and careful with your data</h2>
				<div class="mt-10 grid gap-x-12 gap-y-10 md:grid-cols-3">
					<div class="border-t border-border pt-5">
						<h3 class="font-semibold">Open source</h3>
						<p class="mt-2 text-sm leading-relaxed text-muted">
							Every line is on GitHub under the MIT licence. Read how it works, suggest a change or run your own
							copy.
						</p>
						<a
							href={GITHUB_URL}
							target="_blank"
							rel="noopener"
							class="mt-3 inline-flex items-center gap-1.5 text-sm text-muted transition-colors hover:text-fg"
						>
							View on GitHub <Icon name="external" size={13} />
						</a>
					</div>
					<div class="border-t border-border pt-5">
						<h3 class="font-semibold">Only ticket messages</h3>
						<p class="mt-2 text-sm leading-relaxed text-muted">
							It saves what's said in tickets, for transcripts, and ignores the rest of your server. You decide how
							long transcripts are kept.
						</p>
						<a href="/privacy" class="mt-3 inline-block text-sm text-muted transition-colors hover:text-fg">
							Read the privacy page
						</a>
					</div>
					<div class="border-t border-border pt-5">
						<h3 class="font-semibold">Private by default</h3>
						<p class="mt-2 text-sm leading-relaxed text-muted">
							A ticket is visible to the member, your support roles and your server's admins. So is its
							transcript, and nobody else can find it.
						</p>
						<a
							href="/help/transcripts"
							class="mt-3 inline-block text-sm text-muted transition-colors hover:text-fg"
						>
							Who can read transcripts
						</a>
					</div>
				</div>
			</div>
		</section>

		<section class="border-t border-border">
			<div class="mx-auto max-w-6xl px-5 py-20">
				<h2 class="font-display text-4xl font-bold">How it compares</h2>
				<p class="mt-3 max-w-xl text-muted">
					{APP_NAME} against the two bots most servers switch from.
				</p>

				{#snippet cell(value: string | boolean, muted = false)}
					{#if typeof value === 'boolean'}
						<Icon
							name={value ? 'check' : 'x'}
							size={16}
							class={value ? 'text-success' : 'text-subtle'}
						/>
					{:else}
						<span class={muted ? 'text-muted' : ''}>{value}</span>
					{/if}
				{/snippet}

				<div class="mt-10 overflow-x-auto">
					<table class="w-full min-w-[38rem] border-collapse text-sm">
						<thead>
							<tr class="text-left">
								<th class="pb-4 pr-4 align-bottom font-medium text-subtle"></th>
								<th class="rounded-t-xl bg-accent/10 px-4 pt-4 pb-4 align-bottom">
									<div class="font-display text-lg font-bold">{APP_NAME}</div>
									<div class="mt-0.5 text-xs font-normal text-accent">Free & open source</div>
								</th>
								<th class="px-4 pb-4 align-bottom font-medium text-muted">Ticket Tool</th>
								<th class="px-4 pb-4 align-bottom font-medium text-muted">Tickets</th>
							</tr>
						</thead>
						<tbody>
							{#each comparison as row, i (row.label)}
								<tr>
									<td class="border-t border-border py-3.5 pr-4 text-muted">{row.label}</td>
									<td
										class="border-t border-accent/15 bg-accent/10 px-4 py-3.5 font-medium text-fg {i === comparison.length - 1 ? 'rounded-b-xl' : ''}"
									>
										{@render cell(row.tower)}
									</td>
									<td class="border-t border-border px-4 py-3.5">{@render cell(row.a, true)}</td>
									<td class="border-t border-border px-4 py-3.5">{@render cell(row.b, true)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
				<p class="mt-4 text-xs text-subtle">
					Competitor pricing and features as published on their sites; check theirs before switching, since plans
					change.
				</p>
			</div>
		</section>

		<section class="border-t border-border bg-surface">
			<div class="mx-auto grid max-w-6xl items-center gap-10 px-5 py-20 lg:grid-cols-[1fr_1fr]">
				<div>
					<h2 class="font-display text-4xl font-bold text-balance">More room to grow, when you need it</h2>
					<p class="mt-4 max-w-md text-muted">
						The free plan covers most servers. Premium raises the limits for busier ones and drops the "Powered
						by" footer.
					</p>
					<div class="mt-7">
						<a href={user ? '/servers' : loginURL} data-sveltekit-reload={user ? undefined : true} class="btn btn-primary h-11 px-5">
							See premium <Icon name="arrow-right" size={15} />
						</a>
					</div>
				</div>
				<ul class="space-y-3 border-t border-border pt-6 lg:border-t-0 lg:border-l lg:pt-0 lg:pl-10">
					{#each [
						'100 ticket types instead of 25',
						'50 sets of ticket buttons instead of 10',
						'200 saved replies instead of 50',
						'Transcripts kept forever, not just 90 days',
						`No "Powered by ${APP_NAME}" footer on ticket buttons`
					] as benefit (benefit)}
						<li class="flex items-start gap-3 text-sm">
							<span class="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-accent/15 text-accent">
								<Icon name="check" size={12} />
							</span>
							{benefit}
						</li>
					{/each}
				</ul>
			</div>
		</section>

		<section id="faq" class="scroll-mt-16 border-t border-border">
			<div class="mx-auto grid max-w-6xl gap-12 px-5 py-20 lg:grid-cols-[1fr_2fr]">
				<div>
					<h2 class="font-display text-4xl font-bold">Questions</h2>
					<p class="mt-4 text-muted">
						Something else? <a href="/help" class="text-fg underline-offset-4 hover:underline">Browse the guides</a>
						or
						<a href={SUPPORT_URL} target="_blank" rel="noopener" class="text-fg underline-offset-4 hover:underline"
							>ask us</a
						>.
					</p>
				</div>
				<div class="divide-y divide-border border-y border-border">
					{#each faqs as f (f.q)}
						<details class="group">
							<summary
								class="flex cursor-pointer list-none items-center justify-between gap-4 py-4 font-medium transition-colors hover:text-accent [&::-webkit-details-marker]:hidden"
							>
								{f.q}
								<Icon name="plus" size={16} class="text-muted transition-transform group-open:rotate-45" />
							</summary>
							<div class="pb-5 text-sm leading-relaxed text-muted">
								<p class="max-w-2xl">{f.a}</p>
								{#if f.link}
									<a
										href={f.link.href}
										target={external(f.link.href) ? '_blank' : undefined}
										rel={external(f.link.href) ? 'noopener' : undefined}
										class="mt-2 inline-flex items-center gap-1.5 text-fg underline-offset-4 hover:underline"
									>
										{f.link.label} <Icon name="arrow-right" size={13} />
									</a>
								{/if}
							</div>
						</details>
					{/each}
				</div>
			</div>
		</section>

		<section class="border-t border-border">
			<div class="mx-auto flex max-w-6xl flex-col items-start gap-8 px-5 py-24 lg:flex-row lg:items-end lg:justify-between">
				<div>
					<TicketStub number={1} tone="waiting" size="lg" />
					<h2 class="mt-6 max-w-2xl font-display text-5xl leading-[0.95] font-extrabold text-balance sm:text-6xl">
						Your first ticket is a few minutes away.
					</h2>
				</div>
				{@render ctas()}
			</div>
		</section>
	</main>

	<footer class="border-t border-border">
		<div class="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-x-5 gap-y-3 px-5 py-6 text-xs text-subtle">
			<span>© {new Date().getFullYear()} {APP_NAME}</span>
			<div class="flex flex-wrap gap-5">
				<a href="/help" class="transition-colors hover:text-fg">Help</a>
				<a href={SUPPORT_URL} target="_blank" rel="noopener" class="transition-colors hover:text-fg">Get support</a>
				<a href="/privacy" class="transition-colors hover:text-fg">Privacy</a>
				<a href={GITHUB_URL} target="_blank" rel="noopener" class="transition-colors hover:text-fg">
					Open source on GitHub
				</a>
			</div>
		</div>
	</footer>
</div>
