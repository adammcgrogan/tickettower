<script lang="ts">
	import { onMount } from 'svelte';
	import { getMe, loginURL, type TicketType, type User } from '$lib/api';
	import { APP_NAME, SUPPORT_URL } from '$lib/brand';
	import Icon from '$lib/components/Icon.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import Seo from '$lib/components/Seo.svelte';
	import TicketStub from '$lib/components/TicketStub.svelte';
	import HeroDemo from '$lib/components/landing/HeroDemo.svelte';
	import QueueDemo from '$lib/components/landing/QueueDemo.svelte';
	import Walkthrough from '$lib/components/landing/Walkthrough.svelte';
	import { inviteHref } from '$lib/public';

	const GITHUB_URL = 'https://github.com/adammcgrogan/tickettower';

	let user = $state<User | null>(null);
	// This page is prerendered, so the URL is only read in the browser.
	let loginFailed = $state(false);
	let invite = $state(inviteHref('site'));

	onMount(async () => {
		loginFailed = new URLSearchParams(location.search).get('error') === 'login_failed';
		invite = inviteHref('site', location.search);
		user = await getMe().catch(() => null);
	});

	const steps = [
		{
			title: 'Add the bot',
			body: 'Invite it to your server. It asks only for the permissions tickets need.'
		},
		{
			title: 'Answer a few questions',
			body: 'What tickets are for and who handles them. Your ticket types are created for you.'
		},
		{
			title: 'Post your ticket panel',
			body: 'Pick a channel and publish. Members can open tickets straight away.'
		}
	];

	// What's included, grouped by who it's for.
	const audiences: { title: string; items: { title: string; body: string }[] }[] = [
		{
			title: 'For members',
			items: [
				{ title: 'One click to ask for help', body: 'Buttons or a dropdown menu, in any channel you choose.' },
				{ title: 'Questions up front', body: 'Up to five, so nobody starts from scratch.' },
				{ title: 'Private by default', body: 'A channel or thread only they and your team can see.' },
				{ title: 'A say in how it went', body: 'A rating and a comment when the ticket closes.' }
			]
		},
		{
			title: 'For your team',
			items: [
				{ title: 'Knows whose turn it is', body: 'The longest-waiting member rises to the top of the queue.' },
				{ title: 'Reply from the browser', body: 'Posted in Discord under your name and avatar.' },
				{ title: 'Notes and saved replies', body: 'Private notes for the team, and answers you use often.' },
				{ title: 'Quiet tickets close themselves', body: 'A friendly reminder first, then a close if nobody replies.' }
			]
		},
		{
			title: 'For admins',
			items: [
				{ title: 'Response times and ratings', body: 'Busiest hours, each ticket type and each person on the team.' },
				{ title: 'Transcripts', body: 'Every conversation saved, readable like Discord.' },
				{ title: 'A setup check', body: 'Finds missing permissions before members do, with a link to the fix.' },
				{ title: 'Dashboard access by role', body: 'Let moderators read, reply or manage, as you choose.' }
			]
		}
	];

	const premium = [
		'100 ticket types instead of 25',
		'50 ticket panels instead of 10',
		'200 saved replies instead of 50',
		'Transcripts kept forever, not just 90 days',
		`No "Powered by ${APP_NAME}" footer on ticket panels`
	];

	const comparison: { label: string; tower: string | boolean; a: string | boolean; b: string | boolean }[] = [
		{ label: 'Starting price', tower: 'Free', a: 'Free, Premium $7.99/mo', b: 'Free, Premium $2.99/mo' },
		{ label: 'Reply to tickets from a browser', tower: true, a: false, b: 'Premium only' },
		{ label: 'Analytics and response times', tower: true, a: false, b: false },
		{ label: 'Quiet tickets close themselves', tower: true, a: true, b: false }
	];

	const faqs: { q: string; a: string; link?: { href: string; label: string } }[] = [
		{
			q: 'Is it really free?',
			a: 'The free plan covers everything most servers need: transcripts, analytics and replying from the dashboard, with generous limits on ticket types and ticket panels. A paid premium plan raises those limits for larger servers.'
		},
		{
			q: 'How long does setup take?',
			a: 'A few minutes. Add the bot, answer the quick setup questions on the Home page and post your ticket panel. The setup check tells you if a permission is missing.',
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
			a: 'Up to 25 ticket types and 10 ticket panels per server. Each panel holds up to 25 buttons, which is as many as Discord allows in one message.'
		},
		{
			q: "I'm using another ticket bot. Can I switch?",
			a: 'Add it alongside your current bot and post a new ticket panel. Once the old tickets are finished, remove the old panel and bot.'
		},
		{
			q: 'Can I host it myself?',
			a: `${APP_NAME} is open source under the MIT licence. The README explains how to run your own copy.`,
			link: { href: GITHUB_URL, label: 'View on GitHub' }
		}
	];

	const external = (href: string) => href.startsWith('http');
</script>

<Seo
	title="{APP_NAME}: the free ticket bot for Discord"
	description="{APP_NAME} is a free, open source ticket bot for Discord. Members open private tickets with one click, your team sees who is waiting and replies from the dashboard, and every conversation is saved."
	path="/"
/>

{#snippet ctas(size = 'h-11 px-5')}
	<div class="flex flex-col gap-3 sm:flex-row">
		<a href={invite} data-sveltekit-reload class="btn btn-primary {size}">
			Add to Discord
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
				<a href="#pricing" class="btn btn-ghost hidden md:inline-flex">Pricing</a>
				<a href="#faq" class="btn btn-ghost hidden md:inline-flex">FAQ</a>
				<a href="/help" class="btn btn-ghost hidden sm:inline-flex">Help</a>
				{#if user}
					<a href="/me/tickets" class="btn btn-ghost">My tickets</a>
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

		<section class="mx-auto max-w-6xl px-5 pt-14 pb-20 lg:pt-20">
			<div class="grid items-end gap-8 lg:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)] lg:gap-16">
				<h1 class="font-display text-6xl leading-[0.92] font-extrabold text-balance sm:text-7xl lg:text-[5.5rem]">
					Support tickets for Discord, without the clutter.
				</h1>
				<div class="lg:pb-2">
					<p class="max-w-md text-lg text-pretty text-muted">
						Members open a private ticket with one click. Your team sees who is waiting, replies from Discord or the
						browser, and every conversation is kept.
					</p>
					<div class="mt-7">{@render ctas()}</div>
					<p class="mt-5 text-sm text-subtle">Free and open source. No code or config files.</p>
				</div>
			</div>

			<div class="mt-14"><HeroDemo /></div>
		</section>

		<section id="how-it-works" class="scroll-mt-16 border-t border-border">
			<div class="mx-auto max-w-6xl px-5 py-20">
				<h2 class="font-display text-4xl font-bold">A ticket, start to finish</h2>
				<p class="mt-3 max-w-xl text-muted">
					This is what your members and your team see in Discord. Pick a step to follow one along.
				</p>
				<div class="mt-10"><Walkthrough /></div>
			</div>
		</section>

		<section id="features" class="scroll-mt-16 border-t border-border">
			<div class="mx-auto max-w-6xl px-5 py-20">
				<div class="grid items-center gap-12 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)]">
					<div>
						<h2 class="font-display text-4xl font-bold text-balance">Never lose track of who's waiting</h2>
						<p class="mt-4 max-w-md text-muted">
							The inbox knows whose turn it is. Tickets waiting on your team are marked in amber, longest wait first,
							so nothing slips through while everyone assumes someone else has it.
						</p>
						<p class="mt-4 max-w-md text-muted">
							Read the whole conversation, reply, add a private note or close it, without opening Discord. Keyboard
							shortcuts take you from one ticket to the next.
						</p>
					</div>
					<QueueDemo />
				</div>

				<div class="mt-20 grid gap-x-12 gap-y-12 md:grid-cols-3">
					{#each audiences as a (a.title)}
						<div>
							<h3 class="border-b border-border pb-3 font-semibold">{a.title}</h3>
							<dl class="mt-5 space-y-5">
								{#each a.items as f (f.title)}
									<div>
										<dt class="text-sm font-medium">{f.title}</dt>
										<dd class="mt-1 text-sm leading-relaxed text-muted">{f.body}</dd>
									</div>
								{/each}
							</dl>
						</div>
					{/each}
				</div>
			</div>
		</section>

		<section class="border-t border-border bg-surface">
			<div class="mx-auto max-w-6xl px-5 py-20">
				<div class="flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
					<div>
						<h2 class="font-display text-4xl font-bold">Live in a few minutes</h2>
						<p class="mt-3 max-w-xl text-muted">
							No config files, no commands to memorise. Everything is set up in the dashboard.
						</p>
					</div>
					{@render ctas('h-10 px-4')}
				</div>
				<ol class="mt-12 grid gap-x-12 gap-y-10 md:grid-cols-3">
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

		<section id="pricing" class="scroll-mt-16 border-t border-border">
			<div class="mx-auto max-w-6xl px-5 py-20">
				<h2 class="font-display text-4xl font-bold">Free, with room to grow</h2>
				<p class="mt-3 max-w-xl text-muted">
					The free plan covers what most servers need, including transcripts, analytics and replying from the
					dashboard. Premium raises the limits for busier servers.
				</p>

				<div class="mt-10 grid grid-cols-1 gap-10 lg:grid-cols-[minmax(0,1fr)_minmax(0,22rem)] lg:gap-16">
					<div class="min-w-0">
						<h3 class="font-semibold">How it compares</h3>
						<p class="mt-1 text-sm text-muted">{APP_NAME} against the two bots most servers switch from.</p>

						{#snippet cell(value: string | boolean, muted = false)}
							{#if typeof value === 'boolean'}
								<Icon
									name={value ? 'check' : 'x'}
									size={16}
									class={value ? 'text-success' : 'text-subtle'}
								/>
								<span class="sr-only">{value ? 'Yes' : 'No'}</span>
							{:else}
								<span class={muted ? 'text-muted' : ''}>{value}</span>
							{/if}
						{/snippet}

						<div class="relative mt-6 overflow-x-auto">
							<table class="w-full min-w-[36rem] border-collapse text-sm">
								<thead>
									<tr class="text-left">
										<th class="pr-4 pb-3 align-bottom font-medium text-subtle"><span class="sr-only">Feature</span></th>
										<th class="rounded-t-xl bg-accent/10 px-4 pt-3 pb-3 align-bottom">
											<div class="font-display text-lg font-bold whitespace-nowrap">{APP_NAME}</div>
										</th>
										<th class="px-4 pb-3 align-bottom font-medium text-muted">Ticket Tool</th>
										<th class="px-4 pb-3 align-bottom font-medium text-muted">Tickets</th>
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

					<div class="self-start rounded-xl border border-border bg-surface p-6">
						<h3 class="font-semibold">Premium</h3>
						<p class="mt-1 text-sm text-muted">For busier servers. Upgrade any time from the dashboard.</p>
						<ul class="mt-5 space-y-3">
							{#each premium as benefit (benefit)}
								<li class="flex items-start gap-3 text-sm">
									<Icon name="check" size={15} class="mt-0.5 shrink-0 text-success" />
									{benefit}
								</li>
							{/each}
						</ul>
						<a
							href={user ? '/servers' : loginURL}
							data-sveltekit-reload={user ? undefined : true}
							class="btn btn-secondary mt-6 w-full"
						>
							See premium in the dashboard
						</a>
					</div>
				</div>
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
								class="flex cursor-pointer list-none items-center justify-between gap-4 py-4 font-medium transition-colors hover:text-accent-ink [&::-webkit-details-marker]:hidden"
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
				<a href="/terms" class="transition-colors hover:text-fg">Terms</a>
				<a href={GITHUB_URL} target="_blank" rel="noopener" class="transition-colors hover:text-fg">
					Open source on GitHub
				</a>
			</div>
		</div>
	</footer>
</div>
