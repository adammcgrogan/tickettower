<script lang="ts">
	import { onMount } from 'svelte';
	import { flip } from 'svelte/animate';
	import { fly } from 'svelte/transition';
	import type { TicketType } from '$lib/api';
	import { formatDuration } from '$lib/format';
	import PanelPreview from '../PanelPreview.svelte';
	import TicketStub from '../TicketStub.svelte';

	/**
	 * The whole idea in one interaction: press a button on the ticket panel
	 * (Discord, left) and the ticket lands in the team's queue (the
	 * dashboard, right), marked as waiting on the team, with its wait ticking.
	 */
	const types = [
		{ id: 1, name: 'General support', emoji: '💬', button_style: 'primary' },
		{ id: 2, name: 'Billing', emoji: '💳', button_style: 'secondary' },
		{ id: 3, name: 'Report a member', emoji: '🚩', button_style: 'danger' }
	] as TicketType[];

	type DemoTicket = { number: number; name: string; type: string; since: number; waiting: boolean };

	let now = $state(Date.now());
	let next = 42;
	let tickets = $state<DemoTicket[]>([
		{ number: 38, name: 'priya.r', type: 'General support', since: Date.now() - 11 * 60_000, waiting: true },
		{ number: 40, name: 'jonah', type: 'Report a member', since: Date.now() - 6 * 60_000, waiting: true },
		{ number: 39, name: 'mira.k', type: 'General support', since: Date.now() - 20 * 60_000, waiting: false },
		{ number: 41, name: 'sol', type: 'Billing', since: Date.now() - 9 * 60_000, waiting: false }
	]);
	let pressed = $state(false);
	let announce = $state('');

	// Longest wait first, as in the dashboard; then the member's turn.
	const waiting = $derived(tickets.filter((t) => t.waiting).sort((a, b) => a.since - b.since));
	const theirs = $derived(tickets.filter((t) => !t.waiting));

	function open(t: TicketType, who = 'you') {
		pressed = true;
		const ticket = { number: next++, name: who, type: t.name, since: Date.now(), waiting: true };
		// Keep the demo to a screenful: the oldest ticket on the member's turn goes first.
		const rest = tickets.length >= 5 ? dropOne(tickets) : tickets;
		tickets = [...rest, ticket];
		announce = `Ticket ${ticket.number} opened for ${t.name}. It's waiting on your team.`;
	}

	function dropOne(list: DemoTicket[]) {
		const i = list.findIndex((t) => !t.waiting);
		if (i !== -1) return list.filter((_, j) => j !== i);
		const oldest = [...list].sort((a, b) => a.since - b.since)[0];
		return list.filter((t) => t !== oldest);
	}

	onMount(() => {
		const tickTimer = setInterval(() => (now = Date.now()), 1000);
		// One ticket arrives by itself, to show what happens.
		const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		const first = still ? 0 : setTimeout(() => !pressed && open(types[1], 'alex'), 1600);
		return () => {
			clearInterval(tickTimer);
			clearTimeout(first);
		};
	});

	const age = (t: DemoTicket) => {
		const s = (now - t.since) / 1000;
		return s < 5 ? 'just now' : formatDuration(s);
	};
</script>

<div class="overflow-hidden rounded-2xl border border-border bg-surface shadow-2xl shadow-black/30">
	<div class="grid lg:grid-cols-[minmax(0,1fr)_minmax(0,22rem)]">
		<div class="bg-[#313338]">
			<div class="flex items-center gap-2 border-b border-black/30 px-4 py-2.5 text-sm font-semibold text-[#f2f3f5]">
				<span class="text-lg leading-none text-[#80848e]">#</span> support
				<span class="ml-auto text-xs font-normal text-[#949ba4]">What members see in Discord</span>
			</div>
			<div class="p-1 sm:p-2">
				<PanelPreview
					title="Need a hand?"
					description="Pick a topic and a private ticket opens for you. Our team will be with you shortly."
					color={0xf2b544}
					style="buttons"
					{types}
					onpick={(t) => open(t)}
				/>
			</div>
			<p class="px-5 pb-4 text-xs text-[#949ba4]">
				{pressed ? 'Try another one.' : 'Try it: press a button.'}
			</p>
		</div>

		<div class="flex min-h-80 flex-col border-t border-border lg:border-t-0 lg:border-l">
			<div class="flex items-center justify-between border-b border-border px-4 py-2.5">
				<span class="text-sm font-medium">Your team's inbox</span>
				<span class="text-xs text-subtle">In the dashboard</span>
			</div>
			<ul class="flex-1 text-sm" aria-label="Example ticket queue">
				<li class="flex justify-between border-b border-border bg-bg/40 px-4 py-1.5 text-xs">
					<span class="font-medium text-accent-ink">Waiting on your team</span>
					<span class="text-subtle tabular-nums">{waiting.length}</span>
				</li>
				{#each waiting as t (t.number)}
					<li
						class="flex items-center gap-3 border-b border-border px-4 py-2.5"
						in:fly={{ y: -12, duration: 350 }}
						animate:flip={{ duration: 300 }}
					>
						<TicketStub number={t.number} tone="waiting" size="sm" />
						<span class="min-w-0 flex-1">
							<span class="block truncate font-medium">{t.name}</span>
							<span class="block truncate text-xs text-muted">{t.type}</span>
						</span>
						<span class="shrink-0 text-xs font-medium text-accent-ink tabular-nums">{age(t)}</span>
					</li>
				{/each}
				{#if theirs.length}
					<li class="flex justify-between border-b border-border bg-bg/40 px-4 py-1.5 text-xs">
						<span class="font-medium text-muted">Waiting on the member</span>
						<span class="text-subtle tabular-nums">{theirs.length}</span>
					</li>
					{#each theirs as t (t.number)}
						<li class="flex items-center gap-3 border-b border-border px-4 py-2.5 last:border-b-0" animate:flip={{ duration: 300 }}>
							<TicketStub number={t.number} size="sm" />
							<span class="min-w-0 flex-1">
								<span class="block truncate font-medium">{t.name}</span>
								<span class="block truncate text-xs text-muted">{t.type}</span>
							</span>
							<span class="shrink-0 text-xs text-subtle tabular-nums">{age(t)}</span>
						</li>
					{/each}
				{/if}
			</ul>
		</div>
	</div>
	<p class="sr-only" aria-live="polite">{announce}</p>
</div>
