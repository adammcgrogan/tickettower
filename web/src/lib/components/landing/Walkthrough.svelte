<script lang="ts">
	import { onMount } from 'svelte';
	import { APP_NAME } from '$lib/brand';

	/**
	 * One ticket followed from click to rating, drawn as Discord shows it. The
	 * copy mirrors what the bot really posts (welcome embed, close DM). Steps
	 * advance on their own while in view, until someone picks one.
	 */
	const steps = [
		{ title: 'A member picks a topic', body: 'From ticket buttons or a dropdown menu you design.' },
		{ title: 'They answer your questions', body: 'Up to five, asked before the ticket opens.' },
		{ title: 'A private ticket opens', body: 'Your support roles are pinged, with the answers attached.' },
		{ title: 'Your team replies', body: 'In Discord, or from the dashboard in your browser.' },
		{ title: 'Closed, rated and saved', body: 'The member rates it, and the transcript is kept.' }
	];
	const channels = ['help', 'help', 'billing-0042', 'billing-0042', `${APP_NAME}`];

	let active = $state(0);
	let stopped = $state(false);
	let paused = $state(false);
	let visible = $state(false);
	let root = $state<HTMLElement>();
	let tabs = $state<HTMLButtonElement[]>([]);

	onMount(() => {
		if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) stopped = true;
		const io = new IntersectionObserver(([e]) => (visible = e.isIntersecting), { threshold: 0.4 });
		if (root) io.observe(root);
		return () => io.disconnect();
	});

	$effect(() => {
		if (stopped || paused || !visible) return;
		const id = setInterval(() => (active = (active + 1) % steps.length), 6000);
		return () => clearInterval(id);
	});

	function pick(i: number) {
		stopped = true;
		active = i;
	}

	function onkeydown(e: KeyboardEvent) {
		const delta = { ArrowDown: 1, ArrowRight: 1, ArrowUp: -1, ArrowLeft: -1 }[e.key];
		if (!delta) return;
		e.preventDefault();
		pick((active + delta + steps.length) % steps.length);
		tabs[active]?.focus();
	}

	const time = 'Today at 09:41';
</script>

{#snippet botAvatar()}
	<div class="grid size-10 shrink-0 place-items-center rounded-full bg-accent text-on-accent">
		<svg viewBox="0 0 24 24" width="28" height="28" fill="currentColor" aria-hidden="true">
			<path
				fill-rule="evenodd"
				d="M4.5 2.5h3.4v1.7h2.4V2.5h3.4v1.7h2.4V2.5h3.4v5.1a1.5 1.5 0 0 1-1.5 1.5h-.6v12.4H6.6V9.1H6a1.5 1.5 0 0 1-1.5-1.5zm5.4 14.3h4.2v-3a2.1 2.1 0 0 0-4.2 0z"
			/>
		</svg>
	</div>
{/snippet}

{#snippet person(name: string, color: string)}
	<div class="grid size-10 shrink-0 place-items-center rounded-full font-semibold text-white" style="background:{color}">
		{name[0].toUpperCase()}
	</div>
{/snippet}

{#snippet author(name: string, bot = false, color = '#ffffff')}
	<div class="flex items-center gap-2">
		<span class="font-medium" style="color:{color}">{name}</span>
		{#if bot}<span class="rounded bg-[#5865f2] px-1.5 py-px text-[10px] font-semibold text-white">APP</span>{/if}
		<span class="text-xs text-[#949ba4]">{time}</span>
	</div>
{/snippet}

<div
	bind:this={root}
	class="grid gap-8 lg:grid-cols-[18rem_minmax(0,1fr)] lg:gap-12"
	role="presentation"
	onmouseenter={() => (paused = true)}
	onmouseleave={() => (paused = false)}
>
	<div role="tablist" aria-label="Steps" aria-orientation="vertical" tabindex="-1" class="border-l border-border" {onkeydown}>
		{#each steps as s, i (s.title)}
			{@const on = i === active}
			<button
				bind:this={tabs[i]}
				role="tab"
				id="walk-tab-{i}"
				aria-selected={on}
				aria-controls="walk-panel"
				tabindex={on ? 0 : -1}
				onclick={() => pick(i)}
				class="-ml-px block w-full border-l py-3 pl-5 text-left transition-colors {on
					? 'border-fg'
					: 'border-transparent hover:border-border-strong'}"
			>
				<span class="flex items-baseline gap-3">
					<span class="font-display text-lg leading-none font-bold tabular-nums {on ? 'text-fg' : 'text-subtle'}">
						{i + 1}
					</span>
					<span class="text-sm font-medium {on ? 'text-fg' : 'text-muted'}">{s.title}</span>
				</span>
				<span class="mt-1 block pl-7 text-sm text-muted {on ? '' : 'hidden lg:block lg:text-subtle'}">{s.body}</span>
			</button>
		{/each}
	</div>

	<div
		id="walk-panel"
		role="tabpanel"
		aria-labelledby="walk-tab-{active}"
		class="overflow-hidden rounded-2xl border border-border bg-[#313338] font-[system-ui] text-[15px] leading-snug text-[#dbdee1]"
	>
		<div class="flex h-12 items-center gap-2 border-b border-[#26272b] px-4 text-[#f2f3f5]">
			{#if active === 4}
				<span class="text-[#80848e]">@</span>
			{:else}
				<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="#80848e" stroke-width="2" aria-hidden="true">
					<path d="M4 9h16M4 15h16M10 3 8 21M16 3l-2 18" />
				</svg>
			{/if}
			<span class="font-semibold">{channels[active]}</span>
		</div>

		{#key active}
			<div class="rise min-h-[26rem] p-4 sm:p-5">
				{#if active === 0}
					<div class="flex gap-4">
						{@render botAvatar()}
						<div class="min-w-0 flex-1">
							{@render author(APP_NAME, true)}
							<div class="mt-1.5 max-w-[432px] rounded border-l-4 border-[#f2b544] bg-[#2b2d31] py-3 pr-4 pl-3">
								<div class="font-semibold text-white">Need a hand?</div>
								<p class="mt-1.5 text-sm">Pick a topic below and we'll open a private ticket for you.</p>
								<div class="mt-2 text-xs text-[#949ba4]">Powered by {APP_NAME}</div>
							</div>
							<div class="mt-2 max-w-[400px] overflow-hidden rounded border border-[#1e1f22] bg-[#2b2d31]">
								<div class="flex items-center justify-between bg-[#1e1f22] px-3 py-2.5 text-sm text-[#949ba4]">
									Choose a topic…
									<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m18 15-6-6-6 6" /></svg>
								</div>
								{#each [['💬', 'General support', 'Questions about anything else'], ['💳', 'Billing', 'Payments, refunds and invoices'], ['🚩', 'Report a member', 'Tell the moderators privately']] as [emoji, name, desc], i (name)}
									<div class="flex items-center gap-3 px-3 py-2 {i === 1 ? 'bg-[#404249]' : ''}">
										<span>{emoji}</span>
										<div>
											<div class="text-sm text-white">{name}</div>
											<div class="text-xs text-[#949ba4]">{desc}</div>
										</div>
									</div>
								{/each}
							</div>
						</div>
					</div>
				{:else if active === 1}
					<div class="grid min-h-[24rem] place-items-center rounded-lg bg-black/40 p-4">
						<div class="w-full max-w-md rounded-lg bg-[#313338] shadow-2xl shadow-black/60 ring-1 ring-[#26272b]">
							<div class="flex items-center justify-between px-4 pt-4">
								<div class="text-lg font-semibold text-white">Billing</div>
								<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="#b5bac1" stroke-width="2" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12" /></svg>
							</div>
							<div class="space-y-4 p-4">
								<div>
									<div class="text-sm font-semibold text-[#dbdee1]">Order number <span class="text-[#f23f43]">*</span></div>
									<div class="mt-2 rounded bg-[#1e1f22] px-3 py-2.5 text-sm text-white">NW-20931</div>
								</div>
								<div>
									<div class="text-sm font-semibold text-[#dbdee1]">What's going wrong?</div>
									<div class="mt-2 min-h-20 rounded bg-[#1e1f22] px-3 py-2.5 text-sm text-white">
										I was charged twice for my subscription this month.<span class="caret">|</span>
									</div>
								</div>
							</div>
							<div class="flex justify-end gap-3 rounded-b-lg bg-[#2b2d31] px-4 py-3 text-sm">
								<span class="px-3 py-2 text-white">Cancel</span>
								<span class="rounded bg-[#5865f2] px-4 py-2 font-medium text-white">Submit</span>
							</div>
						</div>
					</div>
				{:else if active === 2}
					<div class="flex gap-4">
						{@render botAvatar()}
						<div class="min-w-0 flex-1">
							{@render author(APP_NAME, true)}
							<div class="mt-0.5 flex flex-wrap gap-1">
								<span class="md-mention">@alex</span><span class="md-mention">@Support team</span>
							</div>
							<div class="mt-1.5 max-w-[432px] rounded border-l-4 border-[#f2b544] bg-[#2b2d31] py-3 pr-4 pl-3">
								<div class="font-semibold text-white">Billing · #42</div>
								<p class="mt-1.5 text-sm">
									Thanks for getting in touch, <span class="md-mention">@alex</span>. Someone from
									<span class="md-mention">@Support team</span> will be with you soon.
								</p>
								<div class="mt-2 text-sm">
									<div class="font-semibold text-white">Order number</div>
									<div>NW-20931</div>
								</div>
								<div class="mt-2 text-sm">
									<div class="font-semibold text-white">What's going wrong?</div>
									<div>I was charged twice for my subscription this month.</div>
								</div>
								<div class="mt-2 text-xs text-[#949ba4]">
									Staff can claim this ticket. Either side can close it when you're done.
								</div>
							</div>
							<div class="mt-2 flex gap-2">
								<span class="rounded bg-[#4e5058] px-4 py-1.5 text-sm font-medium text-white">🙋 Claim</span>
								<span class="rounded bg-[#da373c] px-4 py-1.5 text-sm font-medium text-white">🔒 Close</span>
							</div>
						</div>
					</div>
				{:else if active === 3}
					<div class="space-y-5">
						<div class="flex gap-4">
							{@render person('Priya', '#3ba55c')}
							<div class="min-w-0 flex-1">
								{@render author('Priya', false, '#f2b544')}
								<p class="mt-0.5">
									Hi alex! I can see the double charge. I've refunded the second one, so it should be back in your
									account in a few days.
								</p>
							</div>
						</div>
						<div class="flex gap-4">
							{@render person('alex', '#5865f2')}
							<div class="min-w-0 flex-1">
								{@render author('alex')}
								<p class="mt-0.5">That was quick, thank you! Will it happen again next month?</p>
							</div>
						</div>
						<div class="flex gap-4">
							{@render botAvatar()}
							<div class="min-w-0 flex-1">
								{@render author(APP_NAME, true)}
								<div class="mt-1.5 max-w-[432px] rounded border-l-4 border-[#f2b544] bg-[#2b2d31] py-3 pr-4 pl-3">
									<div class="flex items-center gap-2 text-sm font-semibold text-white">
										<span class="grid size-6 place-items-center rounded-full bg-[#eb459e] text-xs">S</span>
										Sam
									</div>
									<p class="mt-1.5 text-sm">
										It won't. We found what caused it and fixed it this morning. Sorry about the hassle!
									</p>
									<div class="mt-2 text-xs text-[#949ba4]">Sent by Northwind staff from the {APP_NAME} Dashboard</div>
								</div>
								<p class="mt-2 text-xs text-[#949ba4]">
									Sam answered from the dashboard, without opening Discord.
								</p>
							</div>
						</div>
					</div>
				{:else}
					<div class="flex gap-4">
						{@render botAvatar()}
						<div class="min-w-0 flex-1">
							{@render author(APP_NAME, true)}
							<div class="mt-1.5 max-w-[432px] rounded border-l-4 border-[#5d5d66] bg-[#2b2d31] py-3 pr-4 pl-3">
								<div class="font-semibold text-white">Ticket closed</div>
								<p class="mt-1.5 text-sm">
									Your <strong class="text-white">Billing</strong> ticket (#42) in
									<strong class="text-white">Northwind</strong> has been closed.<br />
									<strong class="text-white">Reason:</strong> Refund issued
								</p>
								<p class="mt-3 text-sm">How did we do? Rate your experience below.</p>
							</div>
							<div class="mt-2 flex flex-wrap gap-2">
								{#each [1, 2, 3, 4, 5] as n (n)}
									<span
										class="rounded px-3 py-1.5 text-sm font-medium text-white {n === 5
											? 'bg-[#6d6f78] ring-2 ring-white/70'
											: 'bg-[#4e5058]'}">{'★'.repeat(n)}</span
									>
								{/each}
							</div>
							<div class="mt-2">
								<span class="inline-flex items-center gap-1.5 rounded bg-[#4e5058] px-4 py-1.5 text-sm font-medium text-white">
									View transcript
									<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M15 3h6v6M10 14 21 3M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" /></svg>
								</span>
							</div>
						</div>
					</div>
				{/if}
			</div>
		{/key}
	</div>
</div>

<style>
	.rise {
		animation: rise 0.3s ease-out;
	}
	@keyframes rise {
		from {
			opacity: 0;
			transform: translateY(6px);
		}
	}
	.caret {
		margin-left: 1px;
		animation: blink 1s steps(1) infinite;
	}
	@keyframes blink {
		50% {
			opacity: 0;
		}
	}
</style>
