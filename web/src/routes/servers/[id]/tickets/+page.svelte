<script lang="ts">
	import { getContext, tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		api,
		ApiError,
		errorMessage,
		send,
		type Guild,
		type SavedReply,
		type Ticket,
		type TicketType,
		type Transcript
	} from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { discordURL, ticketState, timeAgo } from '$lib/format';
	import { toast } from '$lib/toast.svelte';
	import Dialog from '$lib/components/Dialog.svelte';
	import Field from '$lib/components/Field.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import Segmented from '$lib/components/Segmented.svelte';
	import TicketStub from '$lib/components/TicketStub.svelte';
	import TicketSummary from '$lib/components/TicketSummary.svelte';
	import TranscriptView from '$lib/components/TranscriptView.svelte';

	type Filter = 'open' | 'closed' | 'all';

	const getGuild = getContext<() => Guild>('guild');
	const guild = $derived(getGuild());

	const filters: { value: Filter; label: string }[] = [
		{ value: 'open', label: 'Open' },
		{ value: 'closed', label: 'Closed' },
		{ value: 'all', label: 'All' }
	];

	// The filter and selected ticket live in the URL, so links from Home and
	// the back button work.
	const filter = $derived<Filter>((page.url.searchParams.get('status') as Filter) || 'open');
	const selectedId = $derived(Number(page.url.searchParams.get('t')) || null);

	function hrefWith(changes: Record<string, string | null>) {
		const params = new URLSearchParams(page.url.searchParams);
		for (const [k, v] of Object.entries(changes)) {
			if (v === null) params.delete(k);
			else params.set(k, v);
		}
		const qs = params.toString();
		return page.url.pathname + (qs ? `?${qs}` : '');
	}

	// How many tickets each request loads.
	const PAGE = 100;

	let tickets = $state<Ticket[] | null>(null);
	let error = $state('');
	let loading = $state(false);
	let query = $state('');
	let typeFilter = $state('');
	let types = $state<TicketType[]>([]);
	let savedReplies = $state<SavedReply[]>([]);
	// Whether older tickets are left to load.
	let more = $state(false);
	let loadingMore = $state(false);
	// Numbers each list request, so a slow reply to an earlier search can't
	// replace a newer one.
	let seq = 0;

	// The search waits for a pause in typing before asking the server.
	let search = $state('');
	$effect(() => {
		const q = query.trim();
		const timer = setTimeout(() => (search = q), 300);
		return () => clearTimeout(timer);
	});

	const filtering = $derived(!!search || !!typeFilter);

	// Another server starts from a clean slate.
	$effect(() => {
		const id = guild.id;
		let stale = false;
		tickets = null;
		types = [];
		savedReplies = [];
		typeFilter = '';
		api<TicketType[]>(`/guilds/${id}/ticket-types`)
			.then((t) => {
				if (!stale) types = t;
			})
			.catch(() => {});
		api<SavedReply[]>(`/guilds/${id}/saved-replies`)
			.then((r) => {
				if (!stale) savedReplies = r;
			})
			.catch(() => {});
		return () => {
			stale = true;
		};
	});

	function listURL(before?: number) {
		const params = new URLSearchParams({ status: filter === 'all' ? '' : filter, limit: String(PAGE) });
		if (typeFilter) params.set('type', typeFilter);
		if (search) params.set('q', search);
		if (before) params.set('before', String(before));
		return `/guilds/${guild.id}/tickets?${params}`;
	}

	// Loads the first page. The current list stays on screen, dimmed, until
	// the new one arrives, so searching doesn't flash a skeleton.
	function load() {
		const url = listURL();
		const req = ++seq;
		loading = true;
		error = '';
		api<Ticket[]>(url)
			.then((t) => {
				if (req !== seq) return;
				tickets = t;
				more = t.length === PAGE;
			})
			.catch((e) => {
				if (req === seq) error = errorMessage(e);
			})
			.finally(() => {
				if (req === seq) loading = false;
			});
	}
	$effect(load);

	async function loadMore() {
		const last = tickets?.at(-1);
		if (!last) return;
		const req = seq;
		loadingMore = true;
		try {
			const older = await api<Ticket[]>(listURL(last.id));
			if (req !== seq || !tickets) return;
			tickets = [...tickets, ...older];
			more = older.length === PAGE;
		} catch (e) {
			if (req === seq) toast(errorMessage(e), 'error');
		} finally {
			loadingMore = false;
		}
	}

	const shown = $derived.by(() => {
		const list = tickets ?? [];
		if (filter !== 'open') return list;
		// Tickets waiting on the team come first, longest wait at the top.
		const waiting = list.filter((t) => t.waiting_on_staff);
		waiting.sort((a, b) => a.last_activity_at.localeCompare(b.last_activity_at));
		return [...waiting, ...list.filter((t) => !t.waiting_on_staff)];
	});

	// --- The selected ticket ---

	let detail = $state<Transcript | null>(null);
	let detailError = $state('');
	// Closed tickets don't change, so their transcripts are kept.
	const closedCache = new Map<number, Transcript>();

	$effect(() => {
		const id = selectedId;
		detail = null;
		detailError = '';
		if (!id) return;
		const hit = closedCache.get(id);
		if (hit) {
			detail = hit;
			return;
		}
		let stale = false;
		api<Transcript>(`/transcripts/${id}`)
			.then((d) => {
				if (d.ticket.status === 'closed') closedCache.set(id, d);
				if (!stale) detail = d;
			})
			.catch((e) => {
				if (!stale) detailError = errorMessage(e);
			});
		return () => {
			stale = true;
		};
	});

	// --- Closing from the dashboard ---

	/** Updates the list for a ticket that just closed. */
	function showClosed(updated: Ticket) {
		if (!tickets) return;
		tickets =
			filter === 'open'
				? tickets.filter((x) => x.id !== updated.id)
				: tickets.map((x) => (x.id === updated.id ? updated : x));
	}

	let closeOpen = $state(false);
	let closeReason = $state('');
	let closeError = $state('');
	let closing = $state(false);

	function openClose() {
		closeReason = '';
		closeError = '';
		closeOpen = true;
	}

	async function closeTicket() {
		if (!detail) return;
		const t = detail.ticket;
		closing = true;
		closeError = '';
		try {
			const updated = await api<Ticket>(
				`/guilds/${guild.id}/tickets/${t.id}/close`,
				send('POST', { reason: closeReason })
			);
			detail = { ...detail, ticket: updated };
			closedCache.set(t.id, detail);
			showClosed(updated);
			closeOpen = false;
			toast(`Ticket #${t.number} closed`);
		} catch (e) {
			if (e instanceof ApiError && e.field) closeError = e.message;
			else {
				closeOpen = false;
				toast(errorMessage(e), 'error');
			}
		} finally {
			closing = false;
		}
	}

	// --- Moving to another ticket type ---

	let moveOpen = $state(false);
	let moveTo = $state('');
	let moveError = $state('');
	let moving = $state(false);

	// Tickets can only move between types that open the same way.
	const moveTargets = $derived.by(() => {
		const t = detail?.ticket;
		return t ? types.filter((tt) => tt.mode === t.mode && tt.id !== t.ticket_type_id) : [];
	});

	function openMove() {
		moveTo = moveTargets.length === 1 ? String(moveTargets[0].id) : '';
		moveError = '';
		moveOpen = true;
	}

	async function moveTicket() {
		if (!detail || !moveTo) return;
		const t = detail.ticket;
		moving = true;
		moveError = '';
		try {
			const updated = await api<Ticket>(
				`/guilds/${guild.id}/tickets/${t.id}/move`,
				send('POST', { type_id: Number(moveTo) })
			);
			if (detail?.ticket.id === t.id) detail = { ...detail, ticket: updated };
			if (tickets) tickets = tickets.map((x) => (x.id === t.id ? updated : x));
			moveOpen = false;
			toast(`Ticket #${t.number} moved to ${updated.type_name}`);
		} catch (err) {
			if (err instanceof ApiError && err.status === 409) {
				moveOpen = false;
				toast(err.message, 'info');
				await refreshClosed(t.id);
			} else if (err instanceof ApiError && err.field) moveError = err.message;
			else {
				moveOpen = false;
				toast(errorMessage(err), 'error');
			}
		} finally {
			moving = false;
		}
	}

	// --- Replying from the dashboard ---

	let reply = $state('');
	let replyError = $state('');
	let sending = $state(false);
	let replyBox = $state<HTMLTextAreaElement>();

	// The bot fills these in when the reply is sent (replyText in ticketbot).
	const hasPlaceholders = $derived(/\{(user|username|number|type|server|staff)\}/.test(reply));

	// A saved reply goes in at the cursor, so it can be edited before sending.
	async function insertSaved(id: number) {
		const saved = savedReplies.find((r) => r.id === id);
		if (!saved) return;
		const start = replyBox?.selectionStart ?? reply.length;
		const end = replyBox?.selectionEnd ?? reply.length;
		reply = reply.slice(0, start) + saved.content + reply.slice(end);
		await tick();
		const cursor = start + saved.content.length;
		replyBox?.focus();
		replyBox?.setSelectionRange(cursor, cursor);
	}

	// A draft belongs to the ticket it was written for.
	$effect(() => {
		void selectedId;
		reply = '';
		replyError = '';
	});

	async function sendReply(e?: SubmitEvent) {
		e?.preventDefault();
		if (!detail || sending || !reply.trim()) return;
		const t = detail.ticket;
		sending = true;
		replyError = '';
		try {
			const res = await api<{ message: Transcript['messages'][number]; ticket: Ticket }>(
				`/guilds/${guild.id}/tickets/${t.id}/reply`,
				send('POST', { content: reply })
			);
			if (detail?.ticket.id === t.id) {
				detail = { ...detail, ticket: res.ticket, messages: [...detail.messages, res.message] };
			}
			if (tickets) tickets = tickets.map((x) => (x.id === t.id ? res.ticket : x));
			reply = '';
		} catch (err) {
			if (err instanceof ApiError && err.status === 409) {
				// The channel was deleted in Discord, and the ticket closed.
				toast(err.message, 'info');
				await refreshClosed(t.id);
			} else if (err instanceof ApiError && err.field) replyError = err.message;
			else toast(errorMessage(err), 'error');
		} finally {
			sending = false;
		}
	}

	async function refreshClosed(id: number) {
		try {
			const d = await api<Transcript>(`/transcripts/${id}`);
			closedCache.set(id, d);
			if (detail?.ticket.id === id) detail = d;
			showClosed(d.ticket);
			reply = '';
		} catch {
			// The toast has already explained; the list catches up on reload.
		}
	}

	const emptyText = $derived(
		filter === 'open'
			? { title: 'No open tickets', body: "You're all caught up." }
			: { title: 'No tickets yet', body: 'Tickets show up here once members start opening them.' }
	);
</script>

<svelte:head><title>Tickets · {guild.name} · {APP_NAME}</title></svelte:head>

<PageHeader title="Tickets" description="Everything members have opened. Pick a ticket to read the conversation." />

<div class="mt-6 flex-wrap items-center gap-2 {selectedId ? 'hidden lg:flex' : 'flex'}">
	<Segmented
		options={filters}
		value={filter}
		label="Show tickets"
		onchange={(v) => goto(hrefWith({ status: v === 'open' ? null : v, t: null }), { replaceState: true, noScroll: true })}
	/>
	<label class="relative block w-full sm:w-72">
		<span class="sr-only">Search tickets</span>
		<Icon
			name="search"
			size={15}
			class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle"
		/>
		<input bind:value={query} placeholder="Search by number, member or type" class="input pl-9" />
	</label>
	{#if types.length > 1}
		<select bind:value={typeFilter} class="input w-auto" aria-label="Ticket type">
			<option value="">All ticket types</option>
			{#each types as tt (tt.id)}<option value={String(tt.id)}>{tt.name}</option>{/each}
		</select>
	{/if}
</div>

<div class="mt-4 grid items-start gap-6 lg:grid-cols-[minmax(0,23rem)_minmax(0,1fr)]">
	<div class={selectedId ? 'hidden lg:block' : ''}>
		{#if error}
			<div class="rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm">
				<p class="text-danger">{error}</p>
				<button onclick={load} class="mt-3 text-muted underline-offset-4 hover:text-fg hover:underline">
					Try again
				</button>
			</div>
		{:else if tickets === null}
			<div class="space-y-px overflow-hidden rounded-xl border border-border" aria-busy="true">
				{#each Array(6) as _, i (i)}<div class="h-[84px] animate-pulse bg-surface"></div>{/each}
			</div>
		{:else if tickets.length === 0 && filtering}
			<div class="rounded-xl border border-dashed border-border px-6 py-14 text-center">
				<p class="font-medium">No tickets match</p>
				<p class="mt-1 text-sm text-muted">Try a different search or ticket type.</p>
			</div>
		{:else if tickets.length === 0}
			<div class="rounded-xl border border-dashed border-border px-6 py-14 text-center">
				<p class="font-medium">{emptyText.title}</p>
				<p class="mx-auto mt-1 max-w-xs text-sm text-muted">{emptyText.body}</p>
			</div>
		{:else}
			<ul
				aria-busy={loading}
				class="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface transition-opacity lg:sticky lg:top-6 lg:max-h-[calc(100dvh-3rem)] lg:overflow-y-auto {loading
					? 'opacity-60'
					: ''}"
			>
				{#each shown as t (t.id)}
					{@const state = ticketState(t)}
					{@const active = t.id === selectedId}
					<li>
						<a
							href={hrefWith({ t: String(t.id) })}
							data-sveltekit-noscroll
							data-sveltekit-replacestate
							aria-current={active ? 'true' : undefined}
							class="relative flex gap-3 px-4 py-3 transition-colors {active
								? 'bg-elevated'
								: 'hover:bg-elevated/50'}"
						>
							{#if active}<span class="absolute inset-y-0 left-0 w-0.5 bg-accent"></span>{/if}
							<TicketStub number={t.number} tone={state.tone} size="sm" />
							<div class="min-w-0 flex-1">
								<div class="flex items-baseline justify-between gap-2">
									<span class="truncate text-sm font-medium">{t.opener_name}</span>
									<span class="shrink-0 text-xs text-subtle">
										{timeAgo(t.closed_at ?? t.last_activity_at)}
									</span>
								</div>
								<div class="truncate text-sm text-muted">{t.type_name}</div>
								<div class="mt-1 truncate text-xs {state.tone === 'waiting' ? 'text-accent' : 'text-subtle'}">
									{state.label}{t.claimed_by_name && t.status === 'open' ? `, claimed by ${t.claimed_by_name}` : ''}
								</div>
							</div>
						</a>
					</li>
				{/each}
				{#if more}
					<li class="p-2">
						<button class="btn btn-ghost w-full" onclick={loadMore} disabled={loadingMore}>
							{loadingMore ? 'Loading…' : 'Load older tickets'}
						</button>
					</li>
				{/if}
			</ul>
		{/if}
	</div>

	<div class="min-w-0 {selectedId ? '' : 'hidden lg:block'}">
		{#if !selectedId}
			<div
				class="grid min-h-96 place-items-center rounded-xl border border-dashed border-border px-6 text-center"
			>
				<div>
					<p class="font-medium">Pick a ticket</p>
					<p class="mt-1 text-sm text-muted">Its details and the conversation show up here.</p>
				</div>
			</div>
		{:else}
			<a
				href={hrefWith({ t: null })}
				data-sveltekit-noscroll
				class="mb-4 inline-flex items-center gap-1.5 text-sm text-muted hover:text-fg lg:hidden"
			>
				<Icon name="arrow-left" size={14} /> All tickets
			</a>
			{#if detailError}
				<div class="rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm text-danger">{detailError}</div>
			{:else if !detail}
				<div class="h-40 animate-pulse rounded-xl bg-surface"></div>
				<div class="mt-4 h-80 animate-pulse rounded-xl bg-surface"></div>
			{:else}
				{@const t = detail.ticket}
				<TicketSummary ticket={t}>
					{#snippet actions()}
						{#if t.status === 'open'}
							<a
								href={discordURL(guild.id, t.channel_id)}
								target="_blank"
								rel="noopener"
								class="btn btn-secondary h-8 px-3"
							>
								Open in Discord <Icon name="external" size={13} />
							</a>
							{#if moveTargets.length > 0}
								<button class="btn btn-secondary h-8 px-3" onclick={openMove}>
									<Icon name="arrow-right" size={13} /> Move
								</button>
							{/if}
							<button class="btn btn-secondary h-8 px-3" onclick={openClose}>
								<Icon name="lock" size={13} /> Close ticket
							</button>
						{/if}
						<a href="/transcripts/{t.id}" target="_blank" rel="noopener" class="btn btn-ghost h-8 px-3">
							Transcript page <Icon name="external" size={13} />
						</a>
					{/snippet}
				</TicketSummary>
				<div class="mt-4">
					<TranscriptView
						messages={detail.messages}
						openerId={t.opener_id}
						openerName={t.opener_name}
						roles={detail.roles}
						channels={detail.channels}
					/>
				</div>
				{#if t.status === 'open'}
					<form
						onsubmit={sendReply}
						class="mt-4 rounded-xl border bg-surface p-3 transition-colors focus-within:border-border-strong {replyError
							? 'border-danger/50'
							: 'border-border'}"
					>
						<label for="reply" class="sr-only">Reply to {t.opener_name}</label>
						<textarea
							id="reply"
							rows="3"
							maxlength="2000"
							bind:this={replyBox}
							bind:value={reply}
							placeholder="Reply to {t.opener_name}…"
							aria-invalid={!!replyError}
							onkeydown={(e) => {
								if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) sendReply();
							}}
							class="block w-full resize-y bg-transparent px-1 text-sm outline-none placeholder:text-subtle"
						></textarea>
						<div class="mt-2 flex items-center justify-between gap-3">
							<p class="text-xs {replyError ? 'text-danger' : 'text-subtle'}">
								{replyError ||
									(hasPlaceholders
										? "Placeholders are filled in when it's sent. ⌘ or Ctrl + Enter sends."
										: `${APP_NAME} posts it in the ticket under your name. ⌘ or Ctrl + Enter sends.`)}
							</p>
							<div class="flex shrink-0 items-center gap-2">
								{#if savedReplies.length > 0}
									<select
										class="input h-8 w-auto max-w-44 py-0 text-sm"
										aria-label="Insert a saved reply"
										onchange={(e) => {
											insertSaved(Number(e.currentTarget.value));
											e.currentTarget.value = '';
										}}
									>
										<option value="" selected disabled>Saved replies</option>
										{#each savedReplies as r (r.id)}<option value={String(r.id)}>{r.name}</option>{/each}
									</select>
								{/if}
								<button type="submit" class="btn btn-primary h-8 shrink-0 px-3" disabled={sending || !reply.trim()}>
									<Icon name="send" size={13} />
									{sending ? 'Sending…' : 'Send'}
								</button>
							</div>
						</div>
					</form>
				{/if}
			{/if}
		{/if}
	</div>
</div>

<Dialog
	bind:open={moveOpen}
	title="Move ticket #{detail?.ticket.number ?? ''}"
	description="It moves to the new type's support team{detail?.ticket.mode === 'channel'
		? ' and category'
		: ''}, and a note in the ticket lets everyone know."
>
	<Field
		label="Ticket type"
		for="move-to"
		hint="Tickets can only move to types that also open as {detail?.ticket.mode === 'thread'
			? 'private threads'
			: 'private channels'}."
		error={moveError}
	>
		<select id="move-to" class="input" bind:value={moveTo} aria-invalid={!!moveError}>
			<option value="" disabled>Choose a ticket type</option>
			{#each moveTargets as tt (tt.id)}<option value={String(tt.id)}>{tt.name}</option>{/each}
		</select>
	</Field>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (moveOpen = false)}>Cancel</button>
		<button class="btn btn-primary" onclick={moveTicket} disabled={moving || !moveTo}>
			{moving ? 'Moving…' : 'Move ticket'}
		</button>
	{/snippet}
</Dialog>

<Dialog
	bind:open={closeOpen}
	title="Close ticket #{detail?.ticket.number ?? ''}?"
	description="{detail?.ticket.opener_name ?? 'The member'} is told it's closed and asked to rate it. {detail?.ticket
		.mode === 'thread'
		? 'The thread is archived'
		: 'The channel is deleted after a few seconds'}, and the transcript is kept."
>
	<Field label="Reason" for="close-reason" optional hint="Shown to the member and in the ticket log." error={closeError}>
		<textarea
			id="close-reason"
			class="input"
			rows="3"
			maxlength="500"
			bind:value={closeReason}
			placeholder="e.g. Resolved, refund issued"
			aria-invalid={!!closeError}
		></textarea>
	</Field>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (closeOpen = false)}>Cancel</button>
		<button class="btn btn-primary" onclick={closeTicket} disabled={closing}>
			{closing ? 'Closing…' : 'Close ticket'}
		</button>
	{/snippet}
</Dialog>
