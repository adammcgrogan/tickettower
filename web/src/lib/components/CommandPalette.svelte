<script lang="ts">
	import { tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, atLeast, type Guild, type Ticket } from '$lib/api';
	import { ticketState } from '$lib/format';
	import { setTheme } from '$lib/theme.svelte';
	import GuildIcon from './GuildIcon.svelte';
	import Icon, { type IconName } from './Icon.svelte';
	import TicketStub from './TicketStub.svelte';

	/**
	 * Jump anywhere with the keyboard: a ticket by number, name or something
	 * said in it, any page of the dashboard, or another server. Opened with
	 * ⌘K / Ctrl+K from anywhere in a server's dashboard.
	 */
	let { open = $bindable(false), guild }: { open?: boolean; guild: Guild } = $props();

	type Item = {
		id: string;
		group: string;
		label: string;
		hint?: string;
		icon?: IconName;
		ticket?: Ticket;
		server?: Guild;
		run: () => void;
	};

	let dialog = $state<HTMLDialogElement>();
	let input = $state<HTMLInputElement>();
	let list = $state<HTMLElement>();
	let query = $state('');
	let active = $state(0);
	let tickets = $state<Ticket[]>([]);
	let searching = $state(false);
	let servers = $state<Guild[] | null>(null);
	let seq = 0;

	const base = $derived(`/servers/${guild.id}`);
	const admin = $derived(atLeast(guild.level, 'admin'));

	$effect(() => {
		if (!dialog) return;
		if (open && !dialog.open) {
			query = '';
			active = 0;
			tickets = [];
			dialog.showModal();
			tick().then(() => input?.focus());
			if (servers === null) {
				api<Guild[]>('/guilds')
					.then((g) => (servers = g.filter((s) => s.bot_present)))
					.catch(() => (servers = []));
			}
		} else if (!open && dialog.open) dialog.close();
	});

	// Tickets are searched on the server, after a pause in typing. The newest
	// request wins.
	$effect(() => {
		const q = query.trim().replace(/^#0*/, '');
		if (!open || !q) {
			tickets = [];
			searching = false;
			return;
		}
		const req = ++seq;
		searching = true;
		const timer = setTimeout(async () => {
			try {
				const found = await api<Ticket[]>(
					`/guilds/${guild.id}/tickets?status=&limit=6&q=${encodeURIComponent(q)}`
				);
				if (req === seq) tickets = found.slice(0, 6);
			} catch {
				if (req === seq) tickets = [];
			} finally {
				if (req === seq) searching = false;
			}
		}, 200);
		return () => clearTimeout(timer);
	});

	function go(href: string) {
		open = false;
		goto(href);
	}

	const pages = $derived<Omit<Item, 'group'>[]>([
		{ id: 'home', label: 'Home', icon: 'home', run: () => go(base) },
		{ id: 'tickets', label: 'Tickets', hint: 'Open tickets', icon: 'inbox', run: () => go(`${base}/tickets`) },
		{
			id: 'closed',
			label: 'Closed tickets',
			icon: 'lock',
			run: () => go(`${base}/tickets?status=closed`)
		},
		{ id: 'analytics', label: 'Analytics', icon: 'chart', run: () => go(`${base}/analytics`) },
		...(admin
			? ([
					{ id: 'types', label: 'Ticket types', icon: 'tag', run: () => go(`${base}/ticket-types`) },
					{ id: 'panels', label: 'Ticket panels', icon: 'panel', run: () => go(`${base}/panels`) },
					{ id: 'replies', label: 'Saved replies', icon: 'message', run: () => go(`${base}/replies`) },
					{ id: 'settings', label: 'Settings', icon: 'settings', run: () => go(`${base}/settings`) },
					{
						id: 'new-type',
						label: 'New ticket type',
						icon: 'plus',
						run: () => go(`${base}/ticket-types/new`)
					},
					{ id: 'new-panel', label: 'New ticket panel', icon: 'plus', run: () => go(`${base}/panels/new`) }
				] satisfies Omit<Item, 'group'>[])
			: []),
		{ id: 'theme-light', label: 'Use the light theme', icon: 'sun', run: () => theme('light') },
		{ id: 'theme-dark', label: 'Use the dark theme', icon: 'moon', run: () => theme('dark') },
		{ id: 'theme-system', label: 'Match the system theme', icon: 'monitor', run: () => theme('system') },
		{
			id: 'help',
			label: 'Help guides',
			icon: 'help',
			run: () => {
				open = false;
				window.open('/help', '_blank');
			}
		}
	]);

	function theme(t: 'light' | 'dark' | 'system') {
		setTheme(t);
		open = false;
	}

	const matches = (label: string, q: string) => label.toLowerCase().includes(q);

	const items = $derived.by(() => {
		const q = query.trim().toLowerCase();
		const out: Item[] = [];
		for (const t of tickets) {
			out.push({
				id: `t${t.id}`,
				group: 'Tickets',
				label: t.opener_name,
				hint: t.type_name,
				ticket: t,
				run: () => go(`${base}/tickets?${t.status === 'closed' ? 'status=all&' : ''}t=${t.id}`)
			});
		}
		for (const p of pages) {
			if (!q || matches(p.label, q)) out.push({ ...p, group: q ? 'Pages' : 'Go to' });
		}
		for (const s of servers ?? []) {
			if (s.id === guild.id) continue;
			if (!q || matches(s.name, q)) {
				out.push({
					id: `s${s.id}`,
					group: 'Switch server',
					label: s.name,
					server: s,
					run: () => go(`/servers/${s.id}`)
				});
			}
		}
		return out;
	});

	// Keep the highlight on a real row as results change.
	$effect(() => {
		if (active >= items.length) active = Math.max(0, items.length - 1);
	});

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			active = (active + 1) % Math.max(1, items.length);
			scrollActive();
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			active = (active - 1 + items.length) % Math.max(1, items.length);
			scrollActive();
		} else if (e.key === 'Enter') {
			e.preventDefault();
			items[active]?.run();
		}
	}

	async function scrollActive() {
		await tick();
		list?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' });
	}
</script>

<dialog
	bind:this={dialog}
	onclose={() => (open = false)}
	onclick={(e) => {
		if (e.target === dialog) open = false;
	}}
	aria-label="Search and jump"
	class="mx-auto mt-[12vh] w-[min(100%-2rem,36rem)] rounded-2xl border border-border-strong bg-surface p-0 text-fg shadow-2xl shadow-black/60 backdrop:bg-black/50 backdrop:backdrop-blur-[2px]"
>
	{#if open}
		<div class="flex items-center gap-3 border-b border-border px-4">
			<Icon name="search" class="text-subtle" />
			<input
				bind:this={input}
				bind:value={query}
				{onkeydown}
				oninput={() => (active = 0)}
				placeholder="Find a ticket, page or server"
				aria-label="Find a ticket, page or server"
				role="combobox"
				aria-expanded="true"
				aria-controls="palette-results"
				aria-activedescendant={items[active] ? `palette-${items[active].id}` : undefined}
				autocomplete="off"
				spellcheck="false"
				class="h-14 min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-subtle"
			/>
			{#if searching}<span class="text-xs text-subtle">Searching…</span>{/if}
		</div>

		<ul
			bind:this={list}
			id="palette-results"
			role="listbox"
			aria-label="Results"
			class="max-h-[min(26rem,60vh)] overflow-y-auto p-2"
		>
			{#each items as item, i (item.id)}
				{#if i === 0 || items[i - 1].group !== item.group}
					<li role="presentation" class="px-2.5 pt-2 pb-1 text-xs text-subtle {i > 0 ? 'mt-1' : ''}">
						{item.group}
					</li>
				{/if}
				<li
					id="palette-{item.id}"
					role="option"
					aria-selected={i === active}
					onclick={item.run}
					onkeydown={() => {}}
					onmousemove={() => (active = i)}
					class="flex cursor-pointer items-center gap-3 rounded-lg px-2.5 py-2 text-sm {i === active
						? 'bg-elevated text-fg'
						: 'text-muted'}"
				>
					{#if item.ticket}
						<TicketStub number={item.ticket.number} tone={ticketState(item.ticket).tone} size="sm" />
						<span class="min-w-0 flex-1 truncate">
							<span class="text-fg">{item.label}</span>
							<span class="text-subtle">{item.hint}</span>
						</span>
						<span class="shrink-0 text-xs text-subtle">
							{item.ticket.status === 'closed' ? 'Closed' : 'Open'}
						</span>
					{:else if item.server}
						<GuildIcon name={item.server.name} url={item.server.icon_url} size={22} />
						<span class="min-w-0 flex-1 truncate">{item.label}</span>
					{:else}
						<span class="grid size-[22px] place-items-center">
							<Icon name={item.icon ?? 'arrow-right'} size={15} />
						</span>
						<span class="min-w-0 flex-1 truncate">{item.label}</span>
					{/if}
					{#if i === active}<Icon name="corner-down-left" size={14} class="text-subtle" />{/if}
				</li>
			{:else}
				<li class="px-3 py-8 text-center text-sm text-muted">
					{searching ? 'Searching…' : 'Nothing matches that. Try a ticket number or a name.'}
				</li>
			{/each}
		</ul>

		<div class="flex items-center gap-4 border-t border-border px-4 py-2.5 text-xs text-subtle">
			<span><kbd class="kbd">↑</kbd> <kbd class="kbd">↓</kbd> to move</span>
			<span><kbd class="kbd">Enter</kbd> to open</span>
			<span class="ml-auto"><kbd class="kbd">Esc</kbd> to close</span>
		</div>
	{/if}
</dialog>
