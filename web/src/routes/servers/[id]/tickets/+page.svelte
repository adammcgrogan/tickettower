<script lang="ts">
	import { getContext, tick, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { SvelteSet } from 'svelte/reactivity';
	import {
		api,
		ApiError,
		atLeast,
		canReopen,
		errorMessage,
		MAX_BULK_CLOSE,
		MAX_NOTE,
		overdueAt,
		send,
		snippetParts,
		type BulkCloseResult,
		type Guild,
		type Member,
		type SavedReply,
		type Ticket,
		type TicketMember,
		type TicketNote,
		type TicketType,
		type Transcript,
		type User
	} from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { discordURL, formatDuration, ticketState, timeAgo } from '$lib/format';
	import { toast } from '$lib/toast.svelte';
	import Dialog from '$lib/components/Dialog.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Field from '$lib/components/Field.svelte';
	import Icon, { type IconName } from '$lib/components/Icon.svelte';
	import LoadError from '$lib/components/LoadError.svelte';
	import Rating from '$lib/components/Rating.svelte';
	import TicketStub from '$lib/components/TicketStub.svelte';
	import TranscriptView from '$lib/components/TranscriptView.svelte';

	type Filter = 'open' | 'closed' | 'all';

	const getGuild = getContext<() => Guild>('guild');
	const guild = $derived(getGuild());
	const getUser = getContext<() => User | null>('user');
	const me = $derived(getUser());
	// Viewers can read along but not act.
	const canAct = $derived(atLeast(guild.level, 'support'));

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
		// A link can start the list filtered to one type (from Ticket types).
		typeFilter = untrack(() => page.url.searchParams.get('type') ?? '');
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
				// Only tickets on screen stay selected, so nothing hidden is closed.
				for (const id of picked) if (!t.some((x) => x.id === id)) picked.delete(id);
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

	// Open tickets are grouped by whose turn it is: waiting on the team first,
	// longest wait at the top, then waiting on the member, then on hold.
	const groups = $derived.by(() => {
		const list = tickets ?? [];
		if (filter !== 'open') return [{ label: '', items: list }];
		const waiting = list.filter((t) => t.waiting_on_staff && !t.on_hold);
		waiting.sort((a, b) => (a.waiting_since ?? a.last_activity_at).localeCompare(b.waiting_since ?? b.last_activity_at));
		return [
			{ label: 'Waiting on your team', items: waiting },
			{ label: 'Waiting on the member', items: list.filter((t) => !t.waiting_on_staff && !t.on_hold) },
			{ label: 'On hold', items: list.filter((t) => t.on_hold) }
		].filter((g) => g.items.length > 0);
	});
	const isOverdue = (t: Ticket) => {
		const at = overdueAt(t, types);
		return at !== null && at <= now;
	};

	// Wait times tick along without reloading.
	let now = $state(Date.now());
	$effect(() => {
		const timer = setInterval(() => (now = Date.now()), 30_000);
		return () => clearInterval(timer);
	});
	const waitedFor = (t: Ticket) =>
		formatDuration((now - new Date(t.waiting_since ?? t.last_activity_at).getTime()) / 1000);
	/** A compact age for the list: 4m, 3h, 2d, 5w. */
	function short(iso: string) {
		const s = Math.max(0, (now - new Date(iso).getTime()) / 1000);
		if (s < 60) return 'now';
		if (s < 3600) return `${Math.floor(s / 60)}m`;
		if (s < 86400) return `${Math.floor(s / 3600)}h`;
		if (s < 604800) return `${Math.floor(s / 86400)}d`;
		return `${Math.floor(s / 604800)}w`;
	}
	const fullTime = (iso: string) =>
		new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });

	// The list in the order it's shown, for moving through it with the keyboard
	// and going on to the next ticket after closing one.
	const ordered = $derived(groups.flatMap((g) => g.items));
	const waitingCount = $derived(filter === 'open' ? (groups.find((g) => g.label === 'Waiting on your team')?.items.length ?? 0) : 0);
	const nextUp = $derived(waitingCount > 0 ? ordered[0] : null);

	function select(id: number | null) {
		goto(hrefWith({ t: id === null ? null : String(id) }), { replaceState: true, noScroll: true, keepFocus: true });
	}

	/** The ticket to show after `id` leaves the list: the one below it, else the one above. */
	function neighbour(id: number): Ticket | null {
		const i = ordered.findIndex((x) => x.id === id);
		if (i === -1) return null;
		return ordered[i + 1] ?? ordered[i - 1] ?? null;
	}

	// Keep the selected row in view when moving with the keyboard.
	let queueScroller = $state<HTMLElement>();
	$effect(() => {
		const id = selectedId;
		if (!id || !queueScroller) return;
		tick().then(() => queueScroller?.querySelector(`[data-ticket="${id}"]`)?.scrollIntoView({ block: 'nearest' }));
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

	// The conversation opens at its latest message, and follows new ones
	// while you're reading at the bottom.
	let scroller = $state<HTMLElement>();
	let shown = { id: 0, count: 0 };
	$effect(() => {
		const d = detail;
		const el = scroller;
		if (!d || !el) return;
		const count = d.messages.length + (d.notes?.length ?? 0);
		const fresh = shown.id !== d.ticket.id;
		const grew = count > shown.count;
		const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 120;
		shown = { id: d.ticket.id, count };
		if (fresh || (grew && atBottom)) tick().then(() => (el.scrollTop = el.scrollHeight));
	});

	// New messages and tickets show up without reloading: the list and the
	// open conversation are refreshed quietly while the tab is visible.
	$effect(() => {
		const timer = setInterval(() => {
			if (document.visibilityState !== 'visible') return;
			refreshQuietly();
		}, 20_000);
		return () => clearInterval(timer);
	});

	function refreshQuietly() {
		if (tickets && !loading && !loadingMore && tickets.length <= PAGE) {
			const req = ++seq;
			api<Ticket[]>(listURL())
				.then((t) => {
					if (req !== seq) return;
					tickets = t;
					more = t.length === PAGE;
				})
				.catch(() => {});
		}
		const id = selectedId;
		if (id && detail?.ticket.status === 'open') {
			api<Transcript>(`/transcripts/${id}`)
				.then((d) => {
					if (selectedId === id && detail?.ticket.id === id) detail = d;
				})
				.catch(() => {});
		}
	}

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

	// --- Holding ---
	let holdOpen = $state(false);
	let holdReason = $state('');
	let holdError = $state('');
	let holding = $state(false);

	function openHold() {
		holdReason = '';
		holdError = '';
		holdOpen = true;
	}

	/** Swaps the updated ticket into the list and the open detail. */
	function replaceTicket(updated: Ticket) {
		if (detail) detail = { ...detail, ticket: updated };
		if (tickets) tickets = tickets.map((x) => (x.id === updated.id ? updated : x));
	}

	async function holdTicket() {
		if (!detail) return;
		const t = detail.ticket;
		holding = true;
		holdError = '';
		try {
			const updated = await api<Ticket>(`/guilds/${guild.id}/tickets/${t.id}/hold`, send('POST', { reason: holdReason }));
			replaceTicket(updated);
			holdOpen = false;
			toast(`Ticket #${t.number} on hold`);
		} catch (e) {
			if (e instanceof ApiError && e.field) holdError = e.message;
			else {
				holdOpen = false;
				toast(errorMessage(e), 'error');
			}
		} finally {
			holding = false;
		}
	}

	async function resumeTicket() {
		if (!detail || holding) return;
		const t = detail.ticket;
		holding = true;
		try {
			replaceTicket(await api<Ticket>(`/guilds/${guild.id}/tickets/${t.id}/resume`, send('POST', {})));
			toast(`Ticket #${t.number} resumed`);
		} catch (e) {
			toast(errorMessage(e), 'error');
		} finally {
			holding = false;
		}
	}

	// --- Claiming and assigning ---
	let claiming = $state(false);
	let assignOpen = $state(false);
	let assignQuery = $state('');
	let assignees = $state<Member[]>([]);
	let assignSearching = $state(false);
	let assignError = $state('');
	let assigning = $state<string | null>(null);
	let assignSeq = 0;

	async function claimTicket() {
		if (!detail || claiming) return;
		const t = detail.ticket;
		claiming = true;
		try {
			replaceTicket(await api<Ticket>(`/guilds/${guild.id}/tickets/${t.id}/claim`, send('POST', {})));
			toast(`You claimed ticket #${t.number}`);
		} catch (e) {
			toast(errorMessage(e), 'error');
		} finally {
			claiming = false;
		}
	}

	async function unclaimTicket() {
		if (!detail || claiming) return;
		const t = detail.ticket;
		claiming = true;
		try {
			replaceTicket(await api<Ticket>(`/guilds/${guild.id}/tickets/${t.id}/unclaim`, send('POST', {})));
			toast(`Ticket #${t.number} unclaimed`);
		} catch (e) {
			toast(errorMessage(e), 'error');
		} finally {
			claiming = false;
		}
	}

	function openAssign() {
		assignQuery = '';
		assignees = [];
		assignError = '';
		assignOpen = true;
	}

	// Searches as the lead types, keeping only the latest result.
	$effect(() => {
		const q = assignQuery.trim();
		const t = detail?.ticket;
		if (!assignOpen || !t) return;
		if (!q) {
			assignees = [];
			return;
		}
		const req = ++assignSeq;
		const timer = setTimeout(async () => {
			assignSearching = true;
			try {
				const found = await api<Member[]>(
					`/guilds/${guild.id}/tickets/${t.id}/assignees?q=${encodeURIComponent(q)}`
				);
				if (req === assignSeq) assignees = found;
			} catch (e) {
				if (req === assignSeq) assignError = errorMessage(e);
			} finally {
				if (req === assignSeq) assignSearching = false;
			}
		}, 250);
		return () => clearTimeout(timer);
	});

	async function assignTo(a: Member) {
		if (!detail || assigning) return;
		const t = detail.ticket;
		assigning = a.id;
		assignError = '';
		try {
			replaceTicket(
				await api<Ticket>(`/guilds/${guild.id}/tickets/${t.id}/claim`, send('POST', { user_id: a.id }))
			);
			assignOpen = false;
			toast(`Ticket #${t.number} assigned to ${a.name}`);
		} catch (e) {
			assignError = errorMessage(e);
		} finally {
			assigning = null;
		}
	}

	// --- Adding and removing people, and renaming ---
	let addOpen = $state(false);
	let addQuery = $state('');
	let addResults = $state<Member[]>([]);
	let addSearching = $state(false);
	let addError = $state('');
	let adding = $state<string | null>(null);
	let addSeq = 0;

	let removeOpen = $state(false);
	let members = $state<TicketMember[] | null>(null);
	let membersError = $state('');
	let removing = $state<string | null>(null);

	let renameOpen = $state(false);
	let renameTo = $state('');
	let renameError = $state('');
	let renaming = $state(false);

	function openAdd() {
		addQuery = '';
		addResults = [];
		addError = '';
		addOpen = true;
	}

	// Searches the server's members as the lead types, keeping only the latest result.
	$effect(() => {
		const q = addQuery.trim();
		if (!addOpen) return;
		if (!q) {
			addResults = [];
			return;
		}
		const req = ++addSeq;
		const timer = setTimeout(async () => {
			addSearching = true;
			try {
				const found = await api<Member[]>(`/guilds/${guild.id}/members?q=${encodeURIComponent(q)}`);
				if (req === addSeq) {
					addResults = found;
					addError = '';
				}
			} catch (e) {
				if (req === addSeq) addError = errorMessage(e);
			} finally {
				if (req === addSeq) addSearching = false;
			}
		}, 250);
		return () => clearTimeout(timer);
	});

	async function addMember(m: Member) {
		if (!detail || adding) return;
		const t = detail.ticket;
		adding = m.id;
		addError = '';
		try {
			await api(`/guilds/${guild.id}/tickets/${t.id}/members`, send('POST', { user_id: m.id }));
			addOpen = false;
			toast(`${m.name} added to ticket #${t.number}`);
		} catch (err) {
			if (err instanceof ApiError && err.status === 409) {
				addOpen = false;
				toast(err.message, 'info');
				await refreshClosed(t.id);
			} else addError = errorMessage(err);
		} finally {
			adding = null;
		}
	}

	async function openRemove() {
		if (!detail) return;
		const t = detail.ticket;
		members = null;
		membersError = '';
		removeOpen = true;
		try {
			const list = await api<TicketMember[]>(`/guilds/${guild.id}/tickets/${t.id}/members`);
			if (detail?.ticket.id === t.id) members = list;
		} catch (err) {
			if (err instanceof ApiError && err.status === 409) {
				removeOpen = false;
				toast(err.message, 'info');
				await refreshClosed(t.id);
			} else membersError = errorMessage(err);
		}
	}

	async function removeMember(m: TicketMember) {
		if (!detail || removing) return;
		const t = detail.ticket;
		removing = m.id;
		membersError = '';
		try {
			await api(`/guilds/${guild.id}/tickets/${t.id}/members/${m.id}`, send('DELETE'));
			members = members?.filter((x) => x.id !== m.id) ?? null;
			toast(m.name ? `${m.name} removed from ticket #${t.number}` : `Removed from ticket #${t.number}`);
		} catch (err) {
			if (err instanceof ApiError && err.status === 409) {
				removeOpen = false;
				toast(err.message, 'info');
				await refreshClosed(t.id);
			} else membersError = errorMessage(err);
		} finally {
			removing = null;
		}
	}

	function openRename() {
		const t = detail?.ticket;
		// A channel's current name is known; a thread's isn't.
		renameTo = (t && detail?.channels[t.channel_id]) || '';
		renameError = '';
		renameOpen = true;
	}

	async function renameTicket(e?: SubmitEvent) {
		e?.preventDefault();
		if (!detail || renaming || !renameTo.trim()) return;
		const t = detail.ticket;
		renaming = true;
		renameError = '';
		try {
			await api(`/guilds/${guild.id}/tickets/${t.id}/rename`, send('POST', { name: renameTo }));
			renameOpen = false;
			toast(`Ticket #${t.number} renamed`);
		} catch (err) {
			if (err instanceof ApiError && err.status === 409) {
				renameOpen = false;
				toast(err.message, 'info');
				await refreshClosed(t.id);
			} else if (err instanceof ApiError && err.field) renameError = err.message;
			else {
				renameOpen = false;
				toast(errorMessage(err), 'error');
			}
		} finally {
			renaming = false;
		}
	}

	// --- Reopening (threads, and channels the type keeps after closing) ---
	let reopening = $state(false);

	async function reopenTicket() {
		if (!detail || reopening) return;
		const t = detail.ticket;
		reopening = true;
		try {
			const updated = await api<Ticket>(`/guilds/${guild.id}/tickets/${t.id}/reopen`, send('POST', {}));
			closedCache.delete(t.id);
			detail = { ...detail, ticket: updated };
			if (tickets) {
				tickets =
					filter === 'closed'
						? tickets.filter((x) => x.id !== updated.id)
						: tickets.map((x) => (x.id === updated.id ? updated : x));
			}
			toast(`Ticket #${t.number} reopened`);
		} catch (e) {
			toast(errorMessage(e), 'error');
		} finally {
			reopening = false;
		}
	}

	// What closing does to the channel, from the ticket's type.
	const closeEffect = $derived.by(() => {
		const t = detail?.ticket;
		if (!t) return '';
		if (t.mode === 'thread') return 'The thread is archived';
		const tt = types.find((x) => x.id === t.ticket_type_id);
		if (tt?.closed_parent_id) {
			return `The channel is kept, read only, for ${tt.closed_keep_days === 1 ? '1 day' : `${tt.closed_keep_days} days`} so it can be reopened`;
		}
		return 'The channel is deleted after a few seconds';
	});

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
			// In the open queue, go straight on to the next ticket.
			const next = filter === 'open' ? neighbour(t.id) : null;
			detail = { ...detail, ticket: updated };
			closedCache.set(t.id, detail);
			showClosed(updated);
			closeOpen = false;
			toast(`Ticket #${t.number} closed`);
			if (filter === 'open') select(next?.id ?? null);
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

	// --- Closing several at once ---
	// Selecting works on the open list, where the queue groups make "everything
	// waiting on the member" an easy pick.

	let selecting = $state(false);
	const picked = new SvelteSet<number>();
	let bulkOpen = $state(false);
	let bulkReason = $state('');
	let bulkError = $state('');
	let bulkClosing = $state(false);
	let bulkTotal = $state(0);
	let bulkDone = $state(0);

	function stopSelecting() {
		selecting = false;
		picked.clear();
	}

	// Another filter or server ends the selection.
	$effect(() => {
		void filter;
		void guild.id;
		stopSelecting();
	});

	function togglePicked(id: number) {
		if (picked.has(id)) picked.delete(id);
		else picked.add(id);
	}

	const allPicked = (items: Ticket[]) => items.every((t) => picked.has(t.id));

	function toggleGroup(items: Ticket[]) {
		const all = allPicked(items);
		for (const t of items) {
			if (all) picked.delete(t.id);
			else picked.add(t.id);
		}
	}

	function openBulkClose() {
		bulkReason = '';
		bulkError = '';
		bulkTotal = picked.size;
		bulkDone = 0;
		bulkOpen = true;
	}

	// The server closes tickets one at a time and hands back any it didn't
	// reach, so keep sending until none are left.
	async function closeSelected() {
		if (bulkClosing || picked.size === 0) return;
		const queue = [...picked];
		const failed: BulkCloseResult['failed'] = [];
		let closed = 0;
		bulkClosing = true;
		bulkError = '';
		try {
			while (queue.length > 0) {
				const res = await api<BulkCloseResult>(
					`/guilds/${guild.id}/tickets/close`,
					send('POST', { ids: queue.splice(0, MAX_BULK_CLOSE), reason: bulkReason })
				);
				for (const t of res.closed) {
					picked.delete(t.id);
					closedCache.delete(t.id);
					if (detail?.ticket.id === t.id) detail = { ...detail, ticket: t };
					showClosed(t);
				}
				failed.push(...res.failed);
				queue.unshift(...res.pending);
				closed += res.closed.length;
				bulkDone = closed + failed.length;
			}
		} catch (e) {
			if (e instanceof ApiError && e.field) {
				bulkError = e.message;
				return;
			}
			toast(errorMessage(e), 'error');
		} finally {
			bulkClosing = false;
		}
		bulkOpen = false;
		if (closed > 0) toast(closed === 1 ? 'Closed 1 ticket' : `Closed ${closed} tickets`);
		if (failed.length > 0) {
			// The ones that failed stay selected, to try again.
			const n = failed.length === 1 ? '1 ticket' : `${failed.length} tickets`;
			toast(`Couldn't close ${n}: ${failed[0].error}`, 'error');
		} else if (picked.size === 0) {
			stopSelecting();
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

	// --- Merging into another open ticket from the same member ---

	let mergeOpen = $state(false);
	let mergeTo = $state('');
	let mergeError = $state('');
	let merging = $state(false);

	// Only other open tickets from the same member can be merged into.
	const mergeTargets = $derived.by(() => {
		const t = detail?.ticket;
		return t && tickets ? tickets.filter((x) => x.status === 'open' && x.opener_id === t.opener_id && x.id !== t.id) : [];
	});

	function openMerge() {
		mergeTo = mergeTargets.length === 1 ? String(mergeTargets[0].id) : '';
		mergeError = '';
		mergeOpen = true;
	}

	async function mergeTicket() {
		if (!detail || !mergeTo) return;
		const t = detail.ticket;
		const target = mergeTargets.find((x) => String(x.id) === mergeTo);
		merging = true;
		mergeError = '';
		try {
			const updated = await api<Ticket>(
				`/guilds/${guild.id}/tickets/${t.id}/merge`,
				send('POST', { ticket_id: Number(mergeTo) })
			);
			detail = { ...detail, ticket: updated };
			closedCache.set(t.id, detail);
			showClosed(updated);
			mergeOpen = false;
			toast(`Ticket #${t.number} merged into ${target ? `#${target.number}` : 'the other ticket'}`);
			// Carry on in the ticket it was merged into.
			if (filter === 'open' && target) select(target.id);
		} catch (err) {
			if (err instanceof ApiError && err.field) mergeError = err.message;
			else {
				mergeOpen = false;
				toast(errorMessage(err), 'error');
			}
		} finally {
			merging = false;
		}
	}

	// --- Replying from the dashboard ---

	let reply = $state('');
	let replyError = $state('');
	let sending = $state(false);
	let replyBox = $state<HTMLTextAreaElement>();

	// --- Private notes, written in the same box ---
	// A note has its own draft, so switching back to Reply can never send it
	// to the member.
	let note = $state('');
	let noteError = $state('');
	let savingNote = $state(false);
	let composeMode = $state<'reply' | 'note'>('reply');
	const composeModes: { value: 'reply' | 'note'; label: string }[] = [
		{ value: 'reply', label: 'Reply' },
		{ value: 'note', label: 'Private note' }
	];
	// Closed tickets only take notes.
	const noting = $derived(composeMode === 'note' || detail?.ticket.status !== 'open');
	const composeError = $derived(noting ? noteError : replyError);

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

	// Saved replies pop up above the reply box, with a filter once there are many.
	let savedOpen = $state(false);
	let savedQuery = $state('');
	let savedMenu = $state<HTMLElement>();
	let savedSearch = $state<HTMLInputElement>();
	const savedMatches = $derived.by(() => {
		const q = savedQuery.trim().toLowerCase();
		return q
			? savedReplies.filter((r) => r.name.toLowerCase().includes(q) || r.content.toLowerCase().includes(q))
			: savedReplies;
	});
	async function openSaved() {
		savedOpen = !savedOpen;
		savedQuery = '';
		if (savedOpen) {
			await tick();
			savedSearch?.focus();
		}
	}
	function pickSaved(id: number) {
		savedOpen = false;
		insertSaved(id);
	}

	let noteBox = $state<HTMLTextAreaElement>();
	function composeKeys(e: KeyboardEvent) {
		if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submitCompose();
		else if (e.key === 'Escape') (e.currentTarget as HTMLElement).blur();
	}

	// --- The More menu ---
	let moreOpen = $state(false);
	let moreMenu = $state<HTMLElement>();
	const menuItem =
		'flex w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left text-sm text-muted transition-colors hover:bg-border/60 hover:text-fg';
	// What the More menu offers for an open ticket, for people who can act on it.
	// On narrower screens the details panel is tucked away, so everything it
	// does is here too.
	const moreActions = $derived.by(() => {
		const t = detail?.ticket;
		if (!t || t.status !== 'open' || !canAct) return [];
		const items: { icon: IconName; label: string; run: () => void }[] = [];
		if (!t.claimed_by) items.push({ icon: 'check', label: 'Claim', run: claimTicket });
		else if (t.claimed_by === me?.id) items.push({ icon: 'x', label: 'Unclaim', run: unclaimTicket });
		items.push({ icon: 'user', label: 'Assign to someone', run: openAssign });
		if (t.on_hold) items.push({ icon: 'play', label: 'Resume', run: resumeTicket });
		else items.push({ icon: 'pause', label: 'Put on hold', run: openHold });
		items.push(
			{ icon: 'user-plus', label: 'Add someone', run: openAdd },
			{ icon: 'user-minus', label: 'Remove someone', run: openRemove },
			{ icon: 'pencil', label: 'Rename', run: openRename }
		);
		if (moveTargets.length > 0) items.push({ icon: 'arrow-right', label: 'Move to another ticket type', run: openMove });
		if (mergeTargets.length > 0) items.push({ icon: 'merge', label: 'Merge into another ticket', run: openMerge });
		return items;
	});

	// Other tickets from the same member that are in the list.
	const related = $derived.by(() => {
		const t = detail?.ticket;
		return t && tickets ? tickets.filter((x) => x.opener_id === t.opener_id && x.id !== t.id).slice(0, 5) : [];
	});

	// The details panel, as a drawer on narrower screens.
	let detailsOpen = $state(false);

	// A draft belongs to the ticket it was written for.
	$effect(() => {
		void selectedId;
		moreOpen = false;
		savedOpen = false;
		reply = '';
		replyError = '';
		note = '';
		noteError = '';
		composeMode = 'reply';
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

	async function addNote() {
		if (!detail || savingNote || !note.trim()) return;
		const t = detail.ticket;
		savingNote = true;
		noteError = '';
		try {
			const saved = await api<TicketNote>(`/guilds/${guild.id}/tickets/${t.id}/notes`, send('POST', { content: note }));
			if (detail?.ticket.id === t.id) {
				detail = { ...detail, notes: [...(detail.notes ?? []), saved] };
				if (closedCache.has(t.id)) closedCache.set(t.id, detail);
			}
			note = '';
		} catch (err) {
			if (err instanceof ApiError && err.field) noteError = err.message;
			else toast(errorMessage(err), 'error');
		} finally {
			savingNote = false;
		}
	}

	function submitCompose(e?: SubmitEvent) {
		e?.preventDefault();
		if (noting) addNote();
		else sendReply();
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

	// --- Keyboard shortcuts ---

	let keysOpen = $state(false);
	let searchBox = $state<HTMLInputElement>();
	const keyList = [
		{ label: 'Next ticket', keys: ['J'] },
		{ label: 'Previous ticket', keys: ['K'] },
		{ label: 'Reply', keys: ['R'] },
		{ label: 'Write a private note', keys: ['N'] },
		{ label: 'Close the ticket', keys: ['E'] },
		{ label: 'Send', keys: ['⌘', 'Enter'] },
		{ label: 'Search tickets', keys: ['/'] },
		{ label: 'Back to the list', keys: ['Esc'] },
		{ label: 'Find anything', keys: ['⌘', 'K'] }
	];

	function step(by: number) {
		if (ordered.length === 0) return;
		const i = ordered.findIndex((x) => x.id === selectedId);
		const next = i === -1 ? (by > 0 ? 0 : ordered.length - 1) : Math.min(ordered.length - 1, Math.max(0, i + by));
		select(ordered[next].id);
	}

	async function focusCompose(mode: 'reply' | 'note') {
		if (!detail || !canAct) return;
		if (detail.ticket.status === 'open') composeMode = mode;
		await tick();
		(noting ? noteBox : replyBox)?.focus();
	}

	function shortcuts(e: KeyboardEvent) {
		if (e.key === 'Escape' && (moreOpen || savedOpen || detailsOpen)) {
			moreOpen = savedOpen = detailsOpen = false;
			return;
		}
		if (e.metaKey || e.ctrlKey || e.altKey || e.defaultPrevented) return;
		const el = e.target as HTMLElement;
		if (el.closest('input, textarea, select, [contenteditable], dialog[open]') || document.querySelector('dialog[open]')) return;
		switch (e.key) {
			case 'j':
				step(1);
				break;
			case 'k':
				step(-1);
				break;
			case 'r':
				focusCompose('reply');
				break;
			case 'n':
				focusCompose('note');
				break;
			case 'e':
				if (detail?.ticket.status === 'open' && canAct) openClose();
				break;
			case '/':
				searchBox?.focus();
				break;
			case '?':
				keysOpen = true;
				break;
			case 'Escape':
				if (selectedId) select(null);
				return;
			default:
				return;
		}
		e.preventDefault();
	}
</script>

<svelte:window
	onclick={(e) => {
		if (moreOpen && moreMenu && !moreMenu.contains(e.target as Node)) moreOpen = false;
		if (savedOpen && savedMenu && !savedMenu.contains(e.target as Node)) savedOpen = false;
	}}
	onkeydown={shortcuts}
/>

<svelte:head><title>Tickets · {guild.name} · {APP_NAME}</title></svelte:head>

<!-- Search results in the Assign and Add dialogs: pick a person to act on. -->
{#snippet people(list: Member[], busyId: string | null, searching: boolean, verb: string, busyVerb: string, pick: (m: Member) => void)}
	<ul class="divide-y divide-border rounded-lg border border-border" aria-busy={searching}>
		{#each list as m (m.id)}
			<li>
				<button
					class="flex w-full items-center gap-3 px-3 py-2 text-left text-sm transition-colors hover:bg-elevated disabled:opacity-60"
					onclick={() => pick(m)}
					disabled={!!busyId}
				>
					<img src={m.avatar_url} alt="" class="size-7 shrink-0 rounded-full bg-elevated" />
					<span class="min-w-0 flex-1 truncate font-medium">{m.name}</span>
					<span class="shrink-0 text-xs text-subtle">{busyId === m.id ? busyVerb : verb}</span>
				</button>
			</li>
		{/each}
	</ul>
{/snippet}

<!-- The facts about the selected ticket, and the actions that change them.
     Beside the conversation on wide screens, in a drawer otherwise. -->
{#snippet details(t: Ticket)}
	{@const state = ticketState(t)}
	<div class="border-b border-border px-5 py-4">
		<p class="text-xs text-subtle">Status</p>
		<p class="mt-1 flex items-center gap-2 text-sm font-medium {state.tone === 'waiting' ? 'text-accent-ink' : ''}">
			<span
				class="size-2 shrink-0 rounded-full {state.tone === 'waiting'
					? isOverdue(t)
						? 'bg-danger'
						: 'bg-accent'
					: state.tone === 'closed'
						? 'bg-subtle'
						: 'bg-success'}"
			></span>
			{state.label}
		</p>
		{#if t.status === 'open' && t.waiting_on_staff && !t.on_hold}
			<p class="mt-1 text-xs {isOverdue(t) ? 'text-danger' : 'text-muted'}">
				{isOverdue(t) ? 'Past the reply target. ' : ''}Waiting {waitedFor(t)}
			</p>
		{/if}
		{#if t.status === 'open' && canAct}
			<div class="mt-3">
				{#if t.on_hold}
					<button class="btn btn-secondary h-8 w-full" onclick={resumeTicket} disabled={holding}>
						<Icon name="play" size={12} />
						{holding ? 'Resuming…' : 'Resume'}
					</button>
				{:else}
					<button class="btn btn-secondary h-8 w-full" onclick={openHold}>
						<Icon name="pause" size={12} /> Put on hold
					</button>
				{/if}
			</div>
		{/if}
	</div>

	<dl class="space-y-4 border-b border-border px-5 py-4 text-sm">
		<div>
			<dt class="text-xs text-subtle">Opened by</dt>
			<dd class="mt-1 truncate">{t.opener_name}</dd>
		</div>
		<div>
			<dt class="flex items-center justify-between text-xs text-subtle">
				Assigned to
				{#if t.status === 'open' && canAct}
					<button class="text-muted hover:text-fg" onclick={openAssign}>Change</button>
				{/if}
			</dt>
			<dd class="mt-1 flex items-center justify-between gap-2">
				<span class="truncate {t.claimed_by_name ? '' : 'text-subtle'}">{t.claimed_by_name ?? 'Nobody yet'}</span>
				{#if t.status === 'open' && canAct}
					{#if !t.claimed_by}
						<button class="btn btn-secondary h-7 px-2.5 text-xs" onclick={claimTicket} disabled={claiming}>
							{claiming ? 'Claiming…' : 'Claim'}
						</button>
					{:else if t.claimed_by === me?.id}
						<button class="text-xs text-muted hover:text-fg" onclick={unclaimTicket} disabled={claiming}>
							{claiming ? 'Unclaiming…' : 'Unclaim'}
						</button>
					{/if}
				{/if}
			</dd>
		</div>
		<div>
			<dt class="flex items-center justify-between text-xs text-subtle">
				Ticket type
				{#if t.status === 'open' && canAct && moveTargets.length > 0}
					<button class="text-muted hover:text-fg" onclick={openMove}>Move</button>
				{/if}
			</dt>
			<dd class="mt-1 truncate">{t.type_name}</dd>
		</div>
		<div>
			<dt class="text-xs text-subtle">{t.mode === 'thread' ? 'Thread' : 'Channel'}</dt>
			<dd class="mt-1 flex items-center justify-between gap-2">
				<span class="truncate">{detail?.channels[t.channel_id] ? `#${detail.channels[t.channel_id]}` : t.mode === 'thread' ? 'Private thread' : 'Private channel'}</span>
				{#if t.status === 'open'}
					<a
						href={discordURL(guild.id, t.channel_id)}
						target="_blank"
						rel="noopener"
						class="inline-flex shrink-0 items-center gap-1 text-xs text-muted hover:text-fg"
					>
						Open in Discord <Icon name="external" size={12} />
					</a>
				{/if}
			</dd>
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<dt class="text-xs text-subtle">Opened</dt>
				<dd class="mt-1" title={fullTime(t.opened_at)}>{timeAgo(t.opened_at)}</dd>
			</div>
			{#if t.status === 'closed'}
				<div>
					<dt class="text-xs text-subtle">Closed</dt>
					<dd class="mt-1" title={t.closed_at ? fullTime(t.closed_at) : ''}>{t.closed_at ? timeAgo(t.closed_at) : 'Yes'}</dd>
				</div>
			{:else}
				<div>
					<dt class="text-xs text-subtle">Last message</dt>
					<dd class="mt-1" title={fullTime(t.last_activity_at)}>{timeAgo(t.last_activity_at)}</dd>
				</div>
			{/if}
		</div>
		{#if t.status === 'closed'}
			<div>
				<dt class="text-xs text-subtle">Closed by</dt>
				<dd class="mt-1">{t.closed_by_name ?? 'The bot'}{t.close_reason ? `: ${t.close_reason}` : ''}</dd>
			</div>
		{/if}
		{#if t.feedback}
			<div>
				<dt class="text-xs text-subtle">Rating</dt>
				<dd class="mt-1.5"><Rating rating={t.feedback.rating} /></dd>
				{#if t.feedback.comment}
					<dd class="mt-1.5 whitespace-pre-wrap text-muted">{t.feedback.comment}</dd>
				{/if}
			</div>
		{/if}
	</dl>

	{#if t.status === 'open' && canAct}
		<div class="border-b border-border px-5 py-4">
			<p class="text-xs text-subtle">People</p>
			<div class="mt-2 flex gap-2">
				<button class="btn btn-secondary h-8 flex-1 px-2.5" onclick={openAdd}>
					<Icon name="user-plus" size={13} /> Add
				</button>
				<button class="btn btn-secondary h-8 flex-1 px-2.5" onclick={openRemove}>
					<Icon name="user-minus" size={13} /> Remove
				</button>
			</div>
		</div>
	{/if}

	{#if related.length > 0}
		<div class="border-b border-border px-5 py-4">
			<p class="flex items-center justify-between text-xs text-subtle">
				Also from {t.opener_name}
				{#if t.status === 'open' && canAct && mergeTargets.length > 0}
					<button class="text-muted hover:text-fg" onclick={openMerge}>Merge</button>
				{/if}
			</p>
			<ul class="mt-2 space-y-1">
				{#each related as r (r.id)}
					<li>
						<a
							href={hrefWith({ t: String(r.id) })}
							data-sveltekit-noscroll
							data-sveltekit-replacestate
							class="-mx-2 flex items-center gap-2.5 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-elevated"
						>
							<TicketStub number={r.number} tone={ticketState(r).tone} size="sm" />
							<span class="min-w-0 flex-1 truncate text-muted">{r.type_name}</span>
							<span class="shrink-0 text-xs text-subtle">{r.status === 'closed' ? 'Closed' : 'Open'}</span>
						</a>
					</li>
				{/each}
			</ul>
		</div>
	{/if}

	<div class="space-y-1 px-3 py-3">
		{#if t.status === 'open' && canAct}
			<button class={menuItem} onclick={openRename}><Icon name="pencil" size={14} /> Rename</button>
		{/if}
		<a href="/transcripts/{t.id}" target="_blank" rel="noopener" class={menuItem}>
			<Icon name="transcript" size={14} /> Open the transcript page
		</a>
	</div>
{/snippet}

<!-- The details panel shows beside the conversation when the workspace
     itself is wide enough (a container query, so the sidebar counts). -->
<div class="@container flex h-[calc(100dvh-3.5rem)] md:h-dvh" style="--queue-w: 22rem" data-workspace>
	<!-- The queue -->
	<section
		aria-label="Tickets"
		class="min-h-0 w-full flex-col border-r border-border bg-bg lg:w-[var(--queue-w)] lg:shrink-0 {selectedId ? 'hidden lg:flex' : 'flex'}"
	>
		<div class="shrink-0 px-4 pt-5 md:pt-6">
			<div class="flex items-center justify-between gap-2">
				<h1 class="font-display text-3xl leading-none font-bold">Tickets</h1>
				<div class="flex items-center gap-1">
					{#if canAct && filter === 'open' && tickets?.length}
						<button
							class="btn h-8 px-2.5 {selecting ? 'btn-secondary' : 'btn-ghost'}"
							onclick={() => (selecting ? stopSelecting() : (selecting = true))}
							aria-pressed={selecting}
						>
							<Icon name="check" size={14} />
							{selecting ? 'Done' : 'Select'}
						</button>
					{/if}
					<button
						class="btn btn-ghost hidden size-8 p-0 md:inline-flex"
						onclick={() => (keysOpen = true)}
						aria-label="Keyboard shortcuts"
						title="Keyboard shortcuts (?)"
					>
						<Icon name="keyboard" size={15} />
					</button>
				</div>
			</div>

			<div role="radiogroup" aria-label="Show tickets" class="mt-4 flex gap-5 border-b border-border">
				{#each filters as f (f.value)}
					<button
						type="button"
						role="radio"
						aria-checked={filter === f.value}
						onclick={() => goto(hrefWith({ status: f.value === 'open' ? null : f.value, t: null }), { replaceState: true, noScroll: true, keepFocus: true })}
						class="-mb-px flex items-center gap-1.5 border-b-2 pb-2.5 text-sm transition-colors {filter === f.value
							? 'border-accent font-medium text-fg'
							: 'border-transparent text-muted hover:text-fg'}"
					>
						{f.label}
						{#if f.value === 'open' && filter === 'open' && tickets && !filtering}
							<span class="text-xs text-subtle tabular-nums">{more ? `${tickets.length}+` : tickets.length}</span>
						{/if}
					</button>
				{/each}
				{#if types.length > 1}
					<select
						bind:value={typeFilter}
						class="input mb-1.5 ml-auto h-7 w-auto max-w-36 min-w-0 self-center border-transparent bg-transparent pl-2 text-sm {typeFilter ? 'text-fg' : 'text-muted'} hover:border-border"
						aria-label="Ticket type"
					>
						<option value="">All types</option>
						{#each types as tt (tt.id)}<option value={String(tt.id)}>{tt.name}</option>{/each}
					</select>
				{/if}
			</div>

			<div class="mt-3 flex gap-2">
				<label class="relative block min-w-0 flex-1">
					<span class="sr-only">Search tickets</span>
					<Icon
						name="search"
						size={15}
						class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle"
					/>
					<input
						bind:this={searchBox}
						bind:value={query}
						placeholder="Search tickets and messages"
						class="input pl-9"
						onkeydown={(e) => {
							if (e.key === 'Escape') {
								query = '';
								searchBox?.blur();
							}
						}}
					/>
				</label>
			</div>
		</div>

		{#if selecting}
			<div class="mx-4 mt-3 flex shrink-0 items-center justify-between gap-3 rounded-lg border border-border bg-surface py-1.5 pr-1.5 pl-3">
				<span class="text-sm {picked.size ? '' : 'text-muted'}" aria-live="polite">
					{picked.size ? `${picked.size} selected` : 'Pick tickets to close'}
				</span>
				<button class="btn btn-primary h-8 px-3" onclick={openBulkClose} disabled={picked.size === 0}>
					<Icon name="lock" size={13} /> Close
				</button>
			</div>
		{/if}

		<div class="mt-3 min-h-0 flex-1 overflow-y-auto" bind:this={queueScroller}>
			{#if error}
				<div class="px-4"><LoadError message={error} onretry={load} /></div>
			{:else if tickets === null}
				<div class="space-y-px" aria-busy="true">
					{#each Array(7) as _, i (i)}
						<div class="flex gap-3 px-4 py-3.5">
							<div class="h-6 w-14 animate-pulse rounded bg-elevated"></div>
							<div class="flex-1 space-y-2">
								<div class="h-3 w-2/3 animate-pulse rounded bg-elevated"></div>
								<div class="h-3 w-1/3 animate-pulse rounded bg-elevated"></div>
							</div>
						</div>
					{/each}
				</div>
			{:else if tickets.length === 0}
				<div class="px-4 pb-4">
					{#if filtering}
						<EmptyState icon="search" title="No tickets match">Try a different search or ticket type.</EmptyState>
					{:else}
						<EmptyState icon="inbox" title={emptyText.title}>{emptyText.body}</EmptyState>
					{/if}
				</div>
			{:else}
				<ul aria-busy={loading} class="pb-4 transition-opacity {loading ? 'opacity-60' : ''}">
					{#each groups as g (g.label)}
						{#if g.label}
							<li
								class="sticky top-0 z-10 flex items-baseline justify-between gap-2 border-y border-border bg-bg/95 px-4 py-1.5 text-xs backdrop-blur first:border-t-0"
							>
								<span class="font-medium {g.label === 'Waiting on your team' ? 'text-accent-ink' : 'text-muted'}">{g.label}</span>
								<span class="flex items-baseline gap-3">
									{#if selecting}
										<button
											class="text-muted underline-offset-4 hover:text-fg hover:underline"
											onclick={() => toggleGroup(g.items)}
										>
											{allPicked(g.items) ? 'Clear' : 'Select all'}
										</button>
									{/if}
									<span class="text-subtle tabular-nums">{g.items.length}</span>
								</span>
							</li>
						{/if}
						{#each g.items as t (t.id)}
							{@const state = ticketState(t)}
							{@const active = t.id === selectedId}
							{@const overdue = isOverdue(t)}
							<li class="flex">
								{#if selecting}
									<label class="flex items-start pt-4 pl-4">
										<span class="sr-only">Select ticket #{t.number}</span>
										<input
											type="checkbox"
											class="size-4 accent-accent"
											checked={picked.has(t.id)}
											onchange={() => togglePicked(t.id)}
										/>
									</label>
								{/if}
								<a
									href={hrefWith({ t: String(t.id) })}
									data-sveltekit-noscroll
									data-sveltekit-replacestate
									data-ticket={t.id}
									aria-current={active ? 'true' : undefined}
									onclick={(e) => {
										if (!selecting) return;
										e.preventDefault();
										togglePicked(t.id);
									}}
									class="relative flex min-w-0 flex-1 gap-3 px-4 py-3 transition-colors {active
										? 'bg-elevated'
										: 'hover:bg-surface'}"
								>
									{#if active}<span class="absolute inset-y-0 left-0 w-0.5 bg-accent"></span>{/if}
									<TicketStub number={t.number} tone={state.tone} size="sm" />
									<div class="min-w-0 flex-1">
										<div class="flex items-baseline justify-between gap-2">
											<span class="truncate text-sm font-medium">{t.opener_name}</span>
											{#if t.status === 'open' && t.waiting_on_staff && !t.on_hold}
												<span
													class="shrink-0 text-xs font-medium tabular-nums {overdue ? 'text-danger' : 'text-accent-ink'}"
													title="{overdue ? 'Past the reply target. ' : ''}Waiting since {fullTime(t.waiting_since ?? t.last_activity_at)}"
												>
													{overdue ? 'Overdue, ' : ''}{short(t.waiting_since ?? t.last_activity_at)}
												</span>
											{:else}
												<span class="flex shrink-0 items-center gap-2 text-xs text-subtle tabular-nums">
													{#if t.feedback}<Rating rating={t.feedback.rating} compact />{/if}
													{short(t.closed_at ?? t.last_activity_at)}
												</span>
											{/if}
										</div>
										<div class="mt-0.5 flex items-baseline justify-between gap-2 text-sm">
											<span class="truncate text-muted">{t.type_name}</span>
											{#if t.status === 'open'}
												<span class="shrink-0 truncate text-xs {t.claimed_by_name ? 'text-muted' : 'text-subtle'}">
													{t.claimed_by_name ?? 'Unclaimed'}
												</span>
											{/if}
										</div>
										{#if filter !== 'open' || t.on_hold}
											<div class="mt-1 truncate text-xs {state.tone === 'waiting' ? 'text-accent-ink' : 'text-subtle'}">
												{state.label}
											</div>
										{/if}
										{#if t.match}
											<p class="mt-1.5 line-clamp-2 text-xs text-muted">
												<span class="text-subtle">{t.match.author_name}:</span>
												{#each snippetParts(t.match.snippet) as part, i (i)}{#if part.hit}<mark
															class="rounded-sm bg-accent/20 px-0.5 text-fg">{part.text}</mark
														>{:else}{part.text}{/if}{/each}
											</p>
										{/if}
									</div>
								</a>
							</li>
						{/each}
					{/each}
					{#if more}
						<li class="px-4 pt-2">
							<button class="btn btn-ghost w-full" onclick={loadMore} disabled={loadingMore}>
								{loadingMore ? 'Loading…' : 'Load older tickets'}
							</button>
						</li>
					{/if}
				</ul>
			{/if}
		</div>
	</section>

	<!-- The conversation -->
	<section
		aria-label="Conversation"
		class="min-h-0 min-w-0 flex-1 flex-col {selectedId ? 'flex' : 'hidden lg:flex'}"
	>
		{#if !selectedId}
			<div class="grid flex-1 place-items-center px-6 text-center">
				<div class="max-w-sm">
					{#if nextUp}
						<p class="font-display text-3xl leading-tight font-bold">
							{waitingCount === 1 ? '1 ticket is' : `${waitingCount} tickets are`} waiting on your team
						</p>
						<p class="mt-2 text-sm text-muted">
							{nextUp.opener_name} has waited longest, for {waitedFor(nextUp)}.
						</p>
						<a
							href={hrefWith({ t: String(nextUp.id) })}
							data-sveltekit-noscroll
							data-sveltekit-replacestate
							class="btn btn-primary mt-5"
						>
							Open #{String(nextUp.number).padStart(4, '0')}
						</a>
					{:else}
						<p class="font-medium">Pick a ticket</p>
						<p class="mt-1 text-sm text-muted">The conversation and everything about it shows up here.</p>
					{/if}
					<p class="mt-8 hidden items-center justify-center gap-2 text-xs text-subtle md:flex">
						<kbd class="kbd">J</kbd><kbd class="kbd">K</kbd> move between tickets
						<span class="mx-1 text-border-strong">|</span>
						<kbd class="kbd">?</kbd> all shortcuts
					</p>
				</div>
			</div>
		{:else if detailError}
			<div class="p-5">
				<a href={hrefWith({ t: null })} data-sveltekit-noscroll class="mb-4 inline-flex items-center gap-1.5 text-sm text-muted hover:text-fg lg:hidden">
					<Icon name="arrow-left" size={14} /> All tickets
				</a>
				<LoadError message={detailError} />
			</div>
		{:else if !detail}
			<div class="flex h-16 shrink-0 items-center gap-3 border-b border-border px-5">
				<div class="h-8 w-20 animate-pulse rounded bg-elevated"></div>
				<div class="h-4 w-40 animate-pulse rounded bg-elevated"></div>
			</div>
			<div class="flex-1 p-5"><div class="h-full animate-pulse rounded-xl bg-surface"></div></div>
		{:else}
			{@const t = detail.ticket}
			{@const state = ticketState(t)}
			<header class="flex shrink-0 items-center gap-3 border-b border-border px-4 py-3 sm:px-5">
				<a
					href={hrefWith({ t: null })}
					data-sveltekit-noscroll
					class="btn btn-ghost -ml-2 size-8 shrink-0 p-0 lg:hidden"
					aria-label="All tickets"
				>
					<Icon name="arrow-left" size={16} />
				</a>
				<TicketStub number={t.number} tone={state.tone} />
				<div class="min-w-0 flex-1">
					<h2 class="truncate text-[15px] leading-tight font-semibold">{t.opener_name}</h2>
					<p class="truncate text-sm {state.tone === 'waiting' ? 'text-accent-ink' : 'text-muted'}">
						{t.type_name}, {state.label.charAt(0).toLowerCase() + state.label.slice(1)}
					</p>
				</div>
				<div class="flex shrink-0 items-center gap-1.5">
					{#if canAct}
						{#if t.status === 'open'}
							{#if !t.claimed_by}
								<button class="btn btn-secondary hidden h-8 px-3 sm:inline-flex" onclick={claimTicket} disabled={claiming}>
									{claiming ? 'Claiming…' : 'Claim'}
								</button>
							{/if}
							<button class="btn btn-secondary h-8 px-3" onclick={openClose} aria-label="Close ticket" title="Close ticket (E)">
								<Icon name="lock" size={13} /> <span class="hidden sm:inline">Close ticket</span>
							</button>
						{:else if canReopen(t)}
							<button class="btn btn-secondary h-8 px-3" onclick={reopenTicket} disabled={reopening}>
								<Icon name="unlock" size={13} /> {reopening ? 'Reopening…' : 'Reopen'}
							</button>
						{/if}
					{/if}
					<div class="relative" bind:this={moreMenu}>
						<button
							class="btn btn-ghost size-8 p-0"
							onclick={() => (moreOpen = !moreOpen)}
							aria-haspopup="menu"
							aria-expanded={moreOpen}
							aria-label="More actions"
							title="More actions"
						>
							<Icon name="more" />
						</button>
						{#if moreOpen}
							<div
								role="menu"
								class="absolute top-full right-0 z-30 mt-1 w-60 rounded-lg border border-border-strong bg-elevated p-1 shadow-xl shadow-black/40"
							>
								{#each moreActions as a (a.label)}
									<button
										role="menuitem"
										class={menuItem}
										onclick={() => {
											moreOpen = false;
											a.run();
										}}
									>
										<Icon name={a.icon} size={14} /> {a.label}
									</button>
								{/each}
								{#if t.status === 'open'}
									<a role="menuitem" href={discordURL(guild.id, t.channel_id)} target="_blank" rel="noopener" class={menuItem}>
										<Icon name="external" size={14} /> Open in Discord
									</a>
								{/if}
								<a role="menuitem" href="/transcripts/{t.id}" target="_blank" rel="noopener" class={menuItem}>
									<Icon name="transcript" size={14} /> Open the transcript page
								</a>
							</div>
						{/if}
					</div>
					<button
						class="btn size-8 p-0 @min-[72rem]:hidden {detailsOpen ? 'btn-secondary' : 'btn-ghost'}"
						onclick={() => (detailsOpen = !detailsOpen)}
						aria-expanded={detailsOpen}
						aria-controls="ticket-details"
						aria-label="Ticket details"
						title="Ticket details"
					>
						<Icon name="panel-right" />
					</button>
				</div>
			</header>

			<div class="flex min-h-0 flex-1">
				<div class="flex min-w-0 flex-1 flex-col">
					<div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-4 py-4 sm:px-5" bind:this={scroller}>
						<!-- Short conversations sit at the bottom, by the reply box, as in Discord. -->
						<div class="mt-auto"></div>
<div class="shrink-0">
						<TranscriptView
							messages={detail.messages}
							notes={detail.notes ?? []}
							openerId={t.opener_id}
							openerName={t.opener_name}
							roles={detail.roles}
							channels={detail.channels}
						/>
</div>
					</div>

					{#if canAct}
						<form onsubmit={submitCompose} class="shrink-0 px-4 pb-4 sm:px-5">
							<div
								class="rounded-xl border bg-surface transition-colors focus-within:border-border-strong {composeError
									? 'border-danger/50'
									: noting
										? 'border-dashed border-border-strong'
										: 'border-border'}"
							>
								{#if t.status === 'open'}
									<div role="radiogroup" aria-label="Write a reply or a private note" class="flex gap-4 px-3.5 pt-2.5">
										{#each composeModes as m (m.value)}
											<button
												type="button"
												role="radio"
												aria-checked={composeMode === m.value}
												onclick={async () => {
													composeMode = m.value;
													await tick();
													(m.value === 'note' ? noteBox : replyBox)?.focus();
												}}
												class="text-sm transition-colors {composeMode === m.value
													? 'font-medium text-fg'
													: 'text-subtle hover:text-muted'}"
											>
												{m.label}
											</button>
										{/each}
									</div>
								{/if}
								{#if noting}
									<label for="note" class="sr-only">Private note on ticket #{t.number}</label>
									<textarea
										id="note"
										rows="2"
										maxlength={MAX_NOTE}
										bind:this={noteBox}
										bind:value={note}
										placeholder={t.status === 'open' ? 'Only your team sees this…' : 'Add a private note for your team…'}
										aria-invalid={!!noteError}
										onkeydown={composeKeys}
										class="block max-h-60 min-h-16 w-full resize-none bg-transparent px-3.5 py-2 text-sm [field-sizing:content] outline-none focus-visible:outline-none placeholder:text-subtle"
									></textarea>
								{:else}
									<label for="reply" class="sr-only">Reply to {t.opener_name}</label>
									<textarea
										id="reply"
										rows="2"
										maxlength="2000"
										bind:this={replyBox}
										bind:value={reply}
										placeholder="Reply to {t.opener_name}…"
										aria-invalid={!!replyError}
										onkeydown={composeKeys}
										class="block max-h-60 min-h-16 w-full resize-none bg-transparent px-3.5 py-2 text-sm [field-sizing:content] outline-none focus-visible:outline-none placeholder:text-subtle"
									></textarea>
								{/if}
								<div class="flex items-center justify-between gap-3 px-2 pb-2 pl-3.5">
									<p class="min-w-0 truncate text-xs {composeError ? 'text-danger' : 'text-subtle'}">
										{composeError ||
											(noting
												? 'The member never sees notes.'
												: hasPlaceholders
													? "Placeholders are filled in when it's sent."
													: `Posted in the ticket under your name.`)}
									</p>
									<div class="flex shrink-0 items-center gap-1.5">
										{#if noting}
											<button type="submit" class="btn btn-primary h-8 shrink-0 px-3" disabled={savingNote || !note.trim()}>
												<Icon name="lock" size={13} />
												{savingNote ? 'Saving…' : 'Add note'}
											</button>
										{:else}
											{#if savedReplies.length > 0}
												<div class="relative" bind:this={savedMenu}>
													<button
														type="button"
														class="btn btn-ghost h-8 px-2.5"
														onclick={openSaved}
														aria-haspopup="menu"
														aria-expanded={savedOpen}
													>
														<Icon name="message" size={14} /> <span class="hidden sm:inline">Saved replies</span>
													</button>
													{#if savedOpen}
														<div
															class="absolute right-0 bottom-full z-30 mb-1 w-80 max-w-[calc(100vw-2rem)] rounded-lg border border-border-strong bg-elevated shadow-xl shadow-black/40"
														>
															{#if savedReplies.length > 5}
																<div class="border-b border-border p-1.5">
																	<input
																		bind:this={savedSearch}
																		bind:value={savedQuery}
																		placeholder="Find a saved reply"
																		aria-label="Find a saved reply"
																		class="h-8 w-full rounded-md bg-transparent px-2 text-sm outline-none placeholder:text-subtle"
																		onkeydown={(e) => {
																			if (e.key === 'Enter' && savedMatches[0]) {
																				e.preventDefault();
																				pickSaved(savedMatches[0].id);
																			}
																		}}
																	/>
																</div>
															{/if}
															<ul role="menu" class="max-h-72 overflow-y-auto p-1">
																{#each savedMatches as r (r.id)}
																	<li>
																		<button
																			type="button"
																			role="menuitem"
																			class="block w-full rounded-md px-2.5 py-2 text-left transition-colors hover:bg-border/60"
																			onclick={() => pickSaved(r.id)}
																		>
																			<span class="block truncate text-sm">{r.name}</span>
																			<span class="block truncate text-xs text-subtle">{r.content}</span>
																		</button>
																	</li>
																{:else}
																	<li class="px-2.5 py-2 text-sm text-muted">No saved reply matches.</li>
																{/each}
															</ul>
														</div>
													{/if}
												</div>
											{/if}
											<button type="submit" class="btn btn-primary h-8 shrink-0 px-3" disabled={sending || !reply.trim()} title="Send (⌘ or Ctrl + Enter)">
												<Icon name="send" size={13} />
												{sending ? 'Sending…' : 'Send'}
											</button>
										{/if}
									</div>
								</div>
							</div>
						</form>
					{/if}
				</div>

				<aside
					id="ticket-details"
					aria-label="Ticket details"
					class="{detailsOpen
						? 'fixed inset-y-0 right-0 z-40 flex w-80 max-w-[90vw] shadow-2xl shadow-black/50'
						: 'hidden'} flex-col overflow-y-auto border-l border-border bg-surface @min-[72rem]:static @min-[72rem]:z-auto @min-[72rem]:flex @min-[72rem]:w-72 @min-[72rem]:max-w-none @min-[72rem]:shrink-0 @min-[72rem]:shadow-none"
				>
					<div class="flex items-center justify-between border-b border-border px-5 py-3 @min-[72rem]:hidden">
						<span class="text-sm font-medium">Ticket #{String(t.number).padStart(4, '0')}</span>
						<button class="btn btn-ghost size-8 p-0" onclick={() => (detailsOpen = false)} aria-label="Close details">
							<Icon name="x" />
						</button>
					</div>
					{@render details(t)}
				</aside>
				{#if detailsOpen}
					<button
						class="fixed inset-0 z-30 bg-black/40 @min-[72rem]:hidden"
						onclick={() => (detailsOpen = false)}
						aria-label="Close details"
						tabindex="-1"
					></button>
				{/if}
			</div>
		{/if}
	</section>
</div>

<Dialog bind:open={keysOpen} title="Keyboard shortcuts" description="They work whenever you're not typing in a box.">
	<dl class="divide-y divide-border text-sm">
		{#each keyList as k (k.label)}
			<div class="flex items-center justify-between gap-4 py-2">
				<dt class="text-muted">{k.label}</dt>
				<dd class="flex gap-1">{#each k.keys as key (key)}<kbd class="kbd">{key}</kbd>{/each}</dd>
			</div>
		{/each}
	</dl>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (keysOpen = false)}>Done</button>
	{/snippet}
</Dialog>

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
	bind:open={mergeOpen}
	title="Merge ticket #{detail?.ticket.number ?? ''}"
	description="This ticket closes and {detail?.ticket.opener_name ?? 'the member'} is told to continue in the ticket you pick. A note links the two, so nothing is lost."
>
	<Field label="Merge into" for="merge-to" error={mergeError}>
		<select id="merge-to" class="input" bind:value={mergeTo} aria-invalid={!!mergeError}>
			<option value="" disabled>Choose a ticket</option>
			{#each mergeTargets as t (t.id)}<option value={String(t.id)}>#{t.number} — {t.type_name}</option>{/each}
		</select>
	</Field>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (mergeOpen = false)}>Cancel</button>
		<button class="btn btn-primary" onclick={mergeTicket} disabled={merging || !mergeTo}>
			{merging ? 'Merging…' : 'Merge ticket'}
		</button>
	{/snippet}
</Dialog>

<Dialog
	bind:open={assignOpen}
	title="Assign ticket #{detail?.ticket.number ?? ''}"
	description="Hand it to someone on the ticket type's support team{detail?.ticket.claimed_by_name
		? `, taking it from ${detail.ticket.claimed_by_name}`
		: ''}. A note in the ticket lets the member know who's helping."
>
	<div class="space-y-3">
		<Field label="Search by name" for="assign-search" error={assignError}>
			<input
				id="assign-search"
				class="input"
				bind:value={assignQuery}
				placeholder="Start typing a name…"
				autocomplete="off"
				aria-invalid={!!assignError}
			/>
		</Field>
		{#if assignQuery.trim()}
			{#if assignees.length === 0}
				<p class="rounded-lg border border-dashed border-border px-4 py-3 text-sm text-muted">
					{assignSearching ? 'Searching…' : 'Nobody on the support team matches that name.'}
				</p>
			{:else}
				{@render people(assignees, assigning, assignSearching, 'Assign', 'Assigning…', assignTo)}
			{/if}
		{/if}
	</div>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (assignOpen = false)}>Cancel</button>
	{/snippet}
</Dialog>

<Dialog
	bind:open={addOpen}
	title="Add someone to ticket #{detail?.ticket.number ?? ''}"
	description="They can read and reply in the ticket, and a note in it mentions them so they find it."
>
	<div class="space-y-3">
		<Field label="Search by name or user ID" for="add-search" error={addError}>
			<input
				id="add-search"
				class="input"
				bind:value={addQuery}
				placeholder="Start typing a name…"
				autocomplete="off"
				aria-invalid={!!addError}
			/>
		</Field>
		{#if addQuery.trim()}
			{#if addResults.length === 0}
				<p class="rounded-lg border border-dashed border-border px-4 py-3 text-sm text-muted">
					{addSearching ? 'Searching…' : 'Nobody in the server matches that.'}
				</p>
			{:else}
				{@render people(addResults, adding, addSearching, 'Add', 'Adding…', addMember)}
			{/if}
		{/if}
	</div>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (addOpen = false)}>Cancel</button>
	{/snippet}
</Dialog>

<Dialog
	bind:open={removeOpen}
	title="Remove someone from ticket #{detail?.ticket.number ?? ''}"
	description={detail?.ticket.mode === 'thread'
		? "They're taken out of the thread, and a note in the ticket says so."
		: 'They lose access to the channel, and a note in the ticket says so. Your support team sees it through its roles, so only people added one by one are listed.'}
>
	<div class="space-y-3">
		{#if membersError}<p class="text-sm text-danger">{membersError}</p>{/if}
		{#if members === null}
			{#if !membersError}
				<div class="space-y-px overflow-hidden rounded-lg border border-border" aria-busy="true">
					{#each Array(2) as _, i (i)}<div class="h-11 animate-pulse bg-elevated/60"></div>{/each}
				</div>
			{/if}
		{:else}
			{#if members.length > 0}
				<ul class="divide-y divide-border rounded-lg border border-border">
					{#each members as m (m.id)}
						<li class="flex items-center gap-3 px-3 py-2 text-sm">
							{#if m.avatar_url}
								<img src={m.avatar_url} alt="" class="size-7 shrink-0 rounded-full bg-elevated" />
							{:else}
								<span class="size-7 shrink-0 rounded-full bg-elevated"></span>
							{/if}
							<span class="min-w-0 flex-1 truncate font-medium {m.name ? '' : 'text-muted'}">
								{m.name || 'Someone who left the server'}
							</span>
							{#if m.opener}
								<span class="shrink-0 text-xs text-subtle">Opened the ticket</span>
							{:else}
								<button
									class="btn btn-ghost h-7 shrink-0 px-2.5 text-xs"
									onclick={() => removeMember(m)}
									disabled={!!removing}
								>
									{removing === m.id ? 'Removing…' : 'Remove'}
								</button>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
			{#if !members.some((m) => !m.opener)}
				<div class="rounded-lg border border-dashed border-border px-4 py-3 text-sm text-muted">
					Nobody else has been added to this ticket.
					<button
						class="text-fg underline-offset-4 hover:underline"
						onclick={() => {
							removeOpen = false;
							openAdd();
						}}
					>
						Add someone
					</button>
				</div>
			{/if}
		{/if}
	</div>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (removeOpen = false)}>Done</button>
	{/snippet}
</Dialog>

<Dialog
	bind:open={renameOpen}
	title="Rename ticket #{detail?.ticket.number ?? ''}"
	description="Changes the name of its {detail?.ticket.mode === 'thread' ? 'thread' : 'channel'} in Discord."
>
	<form id="rename-form" onsubmit={renameTicket}>
		<Field label="Name" for="rename-to" hint="Discord allows two renames every 10 minutes." error={renameError}>
			<input
				id="rename-to"
				class="input"
				bind:value={renameTo}
				maxlength="100"
				placeholder="e.g. refund-order-1234"
				aria-invalid={!!renameError}
			/>
		</Field>
	</form>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (renameOpen = false)}>Cancel</button>
		<button type="submit" form="rename-form" class="btn btn-primary" disabled={renaming || !renameTo.trim()}>
			{renaming ? 'Renaming…' : 'Rename'}
		</button>
	{/snippet}
</Dialog>

<Dialog
	bind:open={holdOpen}
	title="Put ticket #{detail?.ticket.number ?? ''} on hold?"
	description="For tickets waiting on something other than the member or your team. It leaves your queue and won't close for inactivity. The member's next message, or Resume, brings it back."
>
	<Field label="Waiting on" for="hold-reason" optional error={holdError}>
		<input
			id="hold-reason"
			class="input"
			bind:value={holdReason}
			maxlength="200"
			placeholder="e.g. The payment provider's review"
			aria-invalid={!!holdError}
		/>
	</Field>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (holdOpen = false)}>Cancel</button>
		<button class="btn btn-primary" onclick={holdTicket} disabled={holding}>
			{holding ? 'Holding…' : 'Put on hold'}
		</button>
	{/snippet}
</Dialog>

<Dialog
	bind:open={closeOpen}
	title="Close ticket #{detail?.ticket.number ?? ''}?"
	description="{detail?.ticket.opener_name ?? 'The member'} is told it's closed and asked to rate it. {closeEffect}, and the transcript is kept."
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

<Dialog
	bind:open={bulkOpen}
	title="Close {bulkTotal === 1 ? '1 ticket' : `${bulkTotal} tickets`}?"
	description="Each member is told their ticket is closed and asked to rate it, just as when you close one, and the transcripts are kept. They close one after another, so a big batch takes a little while."
>
	<Field label="Reason" for="bulk-close-reason" optional hint="Shown to every member and in the ticket log." error={bulkError}>
		<textarea
			id="bulk-close-reason"
			class="input"
			rows="3"
			maxlength="500"
			bind:value={bulkReason}
			placeholder="e.g. Closing tickets with no reply for a week"
			aria-invalid={!!bulkError}
		></textarea>
	</Field>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (bulkOpen = false)} disabled={bulkClosing}>Cancel</button>
		<button class="btn btn-primary" onclick={closeSelected} disabled={bulkClosing}>
			{bulkClosing
				? `Closing… ${bulkDone} of ${bulkTotal}`
				: `Close ${bulkTotal === 1 ? '1 ticket' : `${bulkTotal} tickets`}`}
		</button>
	{/snippet}
</Dialog>
