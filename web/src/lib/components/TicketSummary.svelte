<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { Ticket } from '$lib/api';
	import { ticketState, timeAgo } from '$lib/format';
	import Rating from './Rating.svelte';
	import TicketStub from './TicketStub.svelte';

	/** The header of a ticket: its number, whose turn it is, and the key facts. */
	let {
		ticket: t,
		staff = true,
		actions
	}: { ticket: Ticket; staff?: boolean; actions?: Snippet } = $props();

	const state = $derived(ticketState(t, staff));
	const time = (iso: string) =>
		new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
</script>

<header class="rounded-xl border border-border bg-surface p-5">
	<div class="flex flex-wrap items-center justify-between gap-4">
		<div class="flex min-w-0 items-center gap-4">
			<TicketStub number={t.number} tone={state.tone} size="lg" />
			<div class="min-w-0">
				<h2 class="truncate text-lg font-semibold">{t.type_name}</h2>
				<p class="text-sm {state.tone === 'waiting' ? 'text-accent' : 'text-muted'}">{state.label}</p>
			</div>
		</div>
		{#if actions}<div class="flex flex-wrap gap-2">{@render actions()}</div>{/if}
	</div>

	<dl class="mt-5 grid grid-cols-2 gap-x-6 gap-y-4 border-t border-border pt-4 text-sm sm:grid-cols-4">
		<div class="min-w-0">
			<dt class="text-xs text-muted">Opened by</dt>
			<dd class="mt-0.5 truncate">{t.opener_name}</dd>
		</div>
		<div>
			<dt class="text-xs text-muted">Opened</dt>
			<dd class="mt-0.5" title={time(t.opened_at)}>{timeAgo(t.opened_at)}</dd>
		</div>
		<div class="min-w-0">
			<dt class="text-xs text-muted">Claimed by</dt>
			<dd class="mt-0.5 truncate {t.claimed_by_name ? '' : 'text-subtle'}">{t.claimed_by_name ?? 'Nobody'}</dd>
		</div>
		{#if t.status === 'closed'}
			<div class="min-w-0">
				<dt class="text-xs text-muted">Closed</dt>
				<dd class="mt-0.5 truncate" title={t.closed_at ? time(t.closed_at) : ''}>
					{t.closed_at ? timeAgo(t.closed_at) : 'Yes'}{t.closed_by_name ? ` by ${t.closed_by_name}` : ''}
				</dd>
			</div>
		{:else}
			<div>
				<dt class="text-xs text-muted">Last message</dt>
				<dd class="mt-0.5" title={time(t.last_activity_at)}>{timeAgo(t.last_activity_at)}</dd>
			</div>
		{/if}
		{#if t.close_reason}
			<div class="col-span-full">
				<dt class="text-xs text-muted">Close reason</dt>
				<dd class="mt-0.5">{t.close_reason}</dd>
			</div>
		{/if}
		{#if t.feedback}
			<div class="col-span-full">
				<dt class="text-xs text-muted">Rating</dt>
				<dd class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1">
					<Rating rating={t.feedback.rating} />
					<span class="text-xs text-subtle" title={time(t.feedback.created_at)}>{timeAgo(t.feedback.created_at)}</span>
				</dd>
				{#if t.feedback.comment}
					<dd class="mt-1.5 whitespace-pre-wrap text-muted">{t.feedback.comment}</dd>
				{/if}
			</div>
		{/if}
	</dl>
</header>
