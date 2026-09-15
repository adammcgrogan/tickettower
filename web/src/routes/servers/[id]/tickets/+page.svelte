<script lang="ts">
	import { getContext, tick } from 'svelte';
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
		type Transcript
	} from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { discordURL, ticketState, timeAgo } from '$lib/format';
	import { toast } from '$lib/toast.svelte';
	import Dialog from '$lib/components/Dialog.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Field from '$lib/components/Field.svelte';
	import Icon, { type IconName } from '$lib/components/Icon.svelte';
	import LoadError from '$lib/components/LoadError.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import Rating from '$lib/components/Rating.svelte';
	import Segmented from '$lib/components/Segmented.svelte';
	import TicketStub from '$lib/components/TicketStub.svelte';
	import TicketSummary from '$lib/components/TicketSummary.svelte';
	import TranscriptView from '$lib/components/TranscriptView.svelte';

	type Filter = 'open' | 'closed' | 'all';

	const getGuild = getContext<() => Guild>('guild');
	const guild = $derived(getGuild());
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
		return at !== null && at <= Date.now();
	};

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

	// --- The More menu ---
	let moreOpen = $state(false);
	let moreMenu = $state<HTMLElement>();
	const menuItem =
		'flex w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left text-sm text-muted transition-colors hover:bg-border/60 hover:text-fg';
	// What the More menu offers for an open ticket, for people who can act on it.
	const moreActions = $derived.by(() => {
		const t = detail?.ticket;
		if (!t || t.status !== 'open' || !canAct) return [];
		const items: { icon: IconName; label: string; run: () => void }[] = [
			{ icon: 'user', label: 'Assign to someone', run: openAssign },
			{ icon: 'user-plus', label: 'Add someone', run: openAdd },
			{ icon: 'user-minus', label: 'Remove someone', run: openRemove },
			{ icon: 'pencil', label: 'Rename', run: openRename }
		];
		if (moveTargets.length > 0) items.push({ icon: 'arrow-right', label: 'Move to another ticket type', run: openMove });
		return items;
	});

	// A draft belongs to the ticket it was written for.
	$effect(() => {
		void selectedId;
		moreOpen = false;
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
</script>

<svelte:window
	onclick={(e) => {
		if (moreOpen && moreMenu && !moreMenu.contains(e.target as Node)) moreOpen = false;
	}}
	onkeydown={(e) => {
		if (e.key === 'Escape') moreOpen = false;
	}}
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
		<input bind:value={query} placeholder="Search tickets and messages" class="input pl-9" />
	</label>
	{#if types.length > 1}
		<select bind:value={typeFilter} class="input w-auto" aria-label="Ticket type">
			<option value="">All ticket types</option>
			{#each types as tt (tt.id)}<option value={String(tt.id)}>{tt.name}</option>{/each}
		</select>
	{/if}
	{#if canAct && filter === 'open' && tickets?.length}
		<button
			class="btn btn-secondary ml-auto"
			onclick={() => (selecting ? stopSelecting() : (selecting = true))}
			aria-pressed={selecting}
		>
			<Icon name="check" size={14} />
			{selecting ? 'Done selecting' : 'Select'}
		</button>
	{/if}
</div>

<div class="mt-4 grid items-start gap-6 lg:grid-cols-[minmax(0,23rem)_minmax(0,1fr)]">
	<div class={selectedId ? 'hidden lg:block' : ''}>
		{#if error}
			<LoadError message={error} onretry={load} />
		{:else if tickets === null}
			<div class="space-y-px overflow-hidden rounded-xl border border-border" aria-busy="true">
				{#each Array(6) as _, i (i)}<div class="h-[84px] animate-pulse bg-surface"></div>{/each}
			</div>
		{:else if tickets.length === 0 && filtering}
			<EmptyState icon="search" title="No tickets match">Try a different search or ticket type.</EmptyState>
		{:else if tickets.length === 0}
			<EmptyState icon="inbox" title={emptyText.title}>{emptyText.body}</EmptyState>
		{:else}
			{#if selecting}
				<div class="mb-2 flex items-center justify-between gap-3 rounded-xl border border-border bg-surface px-4 py-2">
					<span class="text-sm {picked.size ? '' : 'text-muted'}" aria-live="polite">
						{picked.size ? `${picked.size} selected` : 'Pick the tickets to close'}
					</span>
					<button class="btn btn-primary h-8 px-3" onclick={openBulkClose} disabled={picked.size === 0}>
						<Icon name="lock" size={13} /> Close selected
					</button>
				</div>
			{/if}
			<ul
				aria-busy={loading}
				class="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface transition-opacity lg:sticky lg:top-6 lg:max-h-[calc(100dvh-3rem)] lg:overflow-y-auto {loading
					? 'opacity-60'
					: ''}"
			>
				{#each groups as g (g.label)}
					{#if g.label}
						<li class="flex items-baseline justify-between gap-2 bg-bg/50 px-4 py-2 text-xs">
							<span class="font-medium {g.label === 'Waiting on your team' ? 'text-accent' : 'text-muted'}">{g.label}</span>
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
							aria-current={active ? 'true' : undefined}
							onclick={(e) => {
								if (!selecting) return;
								e.preventDefault();
								togglePicked(t.id);
							}}
							class="relative flex min-w-0 flex-1 gap-3 px-4 py-3 transition-colors {active
								? 'bg-elevated'
								: 'hover:bg-elevated/50'}"
						>
							{#if active}<span class="absolute inset-y-0 left-0 w-0.5 bg-accent"></span>{/if}
							<TicketStub number={t.number} tone={state.tone} size="sm" />
							<div class="min-w-0 flex-1">
								<div class="flex items-baseline justify-between gap-2">
									<span class="truncate text-sm font-medium">{t.opener_name}</span>
									<span class="flex shrink-0 items-center gap-2 text-xs text-subtle">
										{#if t.feedback}<Rating rating={t.feedback.rating} compact />{/if}
										{timeAgo(t.closed_at ?? t.last_activity_at)}
									</span>
								</div>
								<div class="truncate text-sm text-muted">{t.type_name}</div>
								<div class="mt-1 truncate text-xs {state.tone === 'waiting' ? 'text-accent' : 'text-subtle'}">
									{#if isOverdue(t)}<span class="font-medium text-danger">Overdue</span>{', '}{/if}{state.label}{t.claimed_by_name && t.status === 'open' ? `, claimed by ${t.claimed_by_name}` : ''}
								</div>
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
				<LoadError message={detailError} />
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
							{#if canAct}
								{#if t.claimed_by}
									<button class="btn btn-secondary h-8 px-3" onclick={unclaimTicket} disabled={claiming}>
										{claiming ? 'Unclaiming…' : 'Unclaim'}
									</button>
								{:else}
									<button class="btn btn-secondary h-8 px-3" onclick={claimTicket} disabled={claiming}>
										{claiming ? 'Claiming…' : 'Claim'}
									</button>
								{/if}
								{#if t.on_hold}
									<button class="btn btn-secondary h-8 px-3" onclick={resumeTicket} disabled={holding}>
										<Icon name="clock" size={13} /> {holding ? 'Resuming…' : 'Resume'}
									</button>
								{:else}
									<button class="btn btn-secondary h-8 px-3" onclick={openHold}>
										<Icon name="clock" size={13} /> Hold
									</button>
								{/if}
								<button class="btn btn-secondary h-8 px-3" onclick={openClose}>
									<Icon name="lock" size={13} /> Close ticket
								</button>
							{/if}
						{:else if canReopen(t) && canAct}
							<button class="btn btn-secondary h-8 px-3" onclick={reopenTicket} disabled={reopening}>
								<Icon name="unlock" size={13} /> {reopening ? 'Reopening…' : 'Reopen'}
							</button>
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
									class="absolute right-0 top-full z-20 mt-1 w-56 rounded-lg border border-border-strong bg-elevated p-1 shadow-xl shadow-black/40"
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
									<a role="menuitem" href="/transcripts/{t.id}" target="_blank" rel="noopener" class={menuItem}>
										<Icon name="transcript" size={14} /> Open the transcript page
									</a>
								</div>
							{/if}
						</div>
					{/snippet}
				</TicketSummary>
				<div class="mt-4">
					<TranscriptView
						messages={detail.messages}
						notes={detail.notes ?? []}
						openerId={t.opener_id}
						openerName={t.opener_name}
						roles={detail.roles}
						channels={detail.channels}
					/>
				</div>
				{#if canAct}
					<form
						onsubmit={submitCompose}
						class="mt-4 rounded-xl border bg-surface p-3 transition-colors focus-within:border-border-strong {composeError
							? 'border-danger/50'
							: noting
								? 'border-dashed border-border-strong'
								: 'border-border'}"
					>
						{#if t.status === 'open'}
							<div class="mb-2">
								<Segmented options={composeModes} bind:value={composeMode} label="Write a reply or a private note" />
							</div>
						{/if}
						{#if noting}
							<label for="note" class="sr-only">Private note on ticket #{t.number}</label>
							<textarea
								id="note"
								rows="3"
								maxlength={MAX_NOTE}
								bind:value={note}
								placeholder="Add a private note for your team…"
								aria-invalid={!!noteError}
								onkeydown={(e) => {
									if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submitCompose();
								}}
								class="block w-full resize-y bg-transparent px-1 text-sm outline-none placeholder:text-subtle"
							></textarea>
						{:else}
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
									if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submitCompose();
								}}
								class="block w-full resize-y bg-transparent px-1 text-sm outline-none placeholder:text-subtle"
							></textarea>
						{/if}
						<div class="mt-2 flex items-center justify-between gap-3">
							<p class="text-xs {composeError ? 'text-danger' : 'text-subtle'}">
								{composeError ||
									(noting
										? 'Only your team sees notes, here and in the transcript. The member never does.'
										: hasPlaceholders
											? "Placeholders are filled in when it's sent. ⌘ or Ctrl + Enter sends."
											: `${APP_NAME} posts it in the ticket under your name. ⌘ or Ctrl + Enter sends.`)}
							</p>
							<div class="flex shrink-0 items-center gap-2">
								{#if noting}
									<button type="submit" class="btn btn-primary h-8 shrink-0 px-3" disabled={savingNote || !note.trim()}>
										<Icon name="lock" size={13} />
										{savingNote ? 'Saving…' : 'Add note'}
									</button>
								{:else}
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
								{/if}
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
