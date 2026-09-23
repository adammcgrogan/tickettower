<script lang="ts">
	import { onMount, setContext } from 'svelte';
	import { page } from '$app/state';
	import { api, atLeast, errorMessage, send, type Guild, type Stats, type Ticket, type User } from '$lib/api';
	import { APP_NAME, SUPPORT_URL } from '$lib/brand';
	import { setTheme, themeState, type ThemePreference } from '$lib/theme.svelte';
	import { toast } from '$lib/toast.svelte';
	import CommandPalette from '$lib/components/CommandPalette.svelte';
	import GuildIcon from '$lib/components/GuildIcon.svelte';
	import Icon, { type IconName } from '$lib/components/Icon.svelte';
	import Logo from '$lib/components/Logo.svelte';

	let { children } = $props();

	let guild = $state<Guild | null>(null);
	let error = $state('');
	let user = $state<User | null>(null);
	let waiting = $state(0);

	let navOpen = $state(false); // the sidebar, on small screens
	let switcherOpen = $state(false);
	let accountOpen = $state(false);
	let paletteOpen = $state(false);
	let servers = $state<Guild[] | null>(null);
	let switcher = $state<HTMLElement>();
	let account = $state<HTMLElement>();
	// Shown in the search button; set on mount, so prerendering doesn't guess.
	let shortcut = $state('Ctrl K');

	setContext('guild', () => guild);
	setContext('user', () => user);

	// Keyed on the ID alone: page.params is a new object on every navigation,
	// and reloading the server each time would remount the page.
	const guildId = $derived(page.params.id);

	$effect(() => {
		const id = guildId;
		guild = null;
		error = '';
		waiting = 0;
		api<Guild>(`/guilds/${id}`)
			.then((g) => (guild = g))
			.catch((e: Error) => (error = e.message));

		// The Tickets badge counts tickets waiting on the team.
		const refresh = () =>
			api<Ticket[]>(`/guilds/${id}/tickets?status=open`)
				.then((t) => (waiting = t.filter((x) => x.waiting_on_staff && !x.on_hold).length))
				.catch(() => {});
		refresh();
		const timer = setInterval(refresh, 60_000);
		return () => clearInterval(timer);
	});

	// The plan, for the upgrade prompt. Only Manage Server can upgrade, so
	// nobody else needs it.
	let plan = $state<Stats | null>(null);
	let checkingOut = $state(false);
	$effect(() => {
		const g = guild;
		plan = null;
		if (!g?.can_manage) return;
		api<Stats>(`/guilds/${g.id}/stats`)
			.then((s) => {
				if (guild?.id === g.id) plan = s;
			})
			.catch(() => {});
	});
	const showUpgrade = $derived(!!plan?.billing_enabled && plan.tier === 'free');

	// Servers that have closed this many tickets have seen enough to review
	// the bot. Asked once per server; "Not now" is remembered in this browser.
	const REVIEW_AFTER = 50;
	let reviewDismissed = $state(true);
	$effect(() => {
		const id = guild?.id;
		if (!id) return;
		try {
			reviewDismissed = localStorage.getItem(`review-dismissed:${id}`) === '1';
		} catch {
			reviewDismissed = false;
		}
	});
	const showReview = $derived(
		!!plan?.review_url && plan.tickets.closed_total >= REVIEW_AFTER && !reviewDismissed
	);
	function dismissReview() {
		reviewDismissed = true;
		try {
			localStorage.setItem(`review-dismissed:${guild?.id}`, '1');
		} catch {
			// Private browsing: it'll ask again next visit.
		}
	}

	async function upgrade() {
		if (!guild) return;
		checkingOut = true;
		try {
			const { url } = await api<{ url: string }>(`/guilds/${guild.id}/billing/checkout`, send('POST'));
			window.location.href = url;
		} catch (err) {
			toast(errorMessage(err), 'error');
			checkingOut = false;
		}
	}

	$effect(() => {
		void page.url.pathname;
		navOpen = false;
		switcherOpen = false;
		accountOpen = false;
	});

	onMount(async () => {
		if (/Mac|iPhone|iPad/.test(navigator.platform)) shortcut = '⌘K';
		user = await api<User>('/me').catch(() => null);
	});

	const base = $derived(`/servers/${page.params.id}`);
	// The current section, kept when switching servers.
	const section = $derived(page.url.pathname.split('/')[3] ?? '');
	// The ticket inbox is a full-height workspace with its own scrolling panes.
	const workspace = $derived(section === 'tickets');

	type NavItem = { label: string; href: string; icon: IconName; badge?: number };
	const primary: NavItem[] = $derived([
		{ label: 'Home', href: base, icon: 'home' },
		{ label: 'Tickets', href: `${base}/tickets`, icon: 'inbox', badge: waiting },
		{ label: 'Analytics', href: `${base}/analytics`, icon: 'chart' }
	]);
	const setup: NavItem[] = $derived([
		{ label: 'Ticket types', href: `${base}/ticket-types`, icon: 'tag' },
		{ label: 'Ticket panels', href: `${base}/panels`, icon: 'panel' },
		{ label: 'Saved replies', href: `${base}/replies`, icon: 'message' },
		{ label: 'Settings', href: `${base}/settings`, icon: 'settings' }
	]);
	const isActive = (href: string) =>
		page.url.pathname === href || (href !== base && page.url.pathname.startsWith(href + '/'));

	function toggleSwitcher() {
		switcherOpen = !switcherOpen;
		if (switcherOpen && servers === null) {
			api<Guild[]>('/guilds')
				.then((g) => (servers = g.filter((s) => s.bot_present)))
				.catch(() => (servers = []));
		}
	}

	const themeOptions: { value: ThemePreference; label: string; icon: IconName }[] = [
		{ value: 'system', label: 'Match system theme', icon: 'monitor' },
		{ value: 'light', label: 'Light theme', icon: 'sun' },
		{ value: 'dark', label: 'Dark theme', icon: 'moon' }
	];

	async function logout() {
		await api('/auth/logout', { method: 'POST' });
		window.location.href = '/';
	}

	const menuItem =
		'flex w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left text-sm text-muted transition-colors hover:bg-border/60 hover:text-fg';
</script>

<svelte:window
	onclick={(e) => {
		if (switcherOpen && switcher && !switcher.contains(e.target as Node)) switcherOpen = false;
		if (accountOpen && account && !account.contains(e.target as Node)) accountOpen = false;
	}}
	onkeydown={(e) => {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k' && guild) {
			e.preventDefault();
			paletteOpen = !paletteOpen;
			return;
		}
		if (e.key === 'Escape') switcherOpen = navOpen = accountOpen = false;
	}}
/>

{#snippet navLink(item: NavItem)}
	{@const active = isActive(item.href)}
	<a
		href={item.href}
		aria-current={active ? 'page' : undefined}
		class="flex items-center gap-3 rounded-lg px-2.5 py-[7px] text-sm transition-colors {active
			? 'bg-elevated font-medium text-fg'
			: 'text-muted hover:bg-elevated/60 hover:text-fg'}"
	>
		<Icon name={item.icon} class={active ? 'text-fg' : 'text-subtle'} />
		<span class="flex-1">{item.label}</span>
		{#if item.badge}
			<span
				class="min-w-5 rounded-full bg-accent px-1.5 text-center text-xs leading-5 font-semibold text-on-accent tabular-nums"
				title="{item.badge} waiting on your team"
			>
				{item.badge}
			</span>
		{/if}
	</a>
{/snippet}

<div class="min-h-dvh md:flex">
	<div
		class="sticky top-0 z-30 flex h-14 items-center gap-1 border-b border-border bg-bg/90 px-4 backdrop-blur md:hidden"
	>
		<a href="/servers" aria-label="All servers" class="mr-auto"><Logo size={26} /></a>
		{#if guild}
			<button class="btn btn-ghost size-9 p-0" onclick={() => (paletteOpen = true)} aria-label="Search">
				<Icon name="search" size={18} />
			</button>
		{/if}
		<button
			class="btn btn-ghost size-9 p-0"
			onclick={() => (navOpen = !navOpen)}
			aria-expanded={navOpen}
			aria-controls="sidebar"
			aria-label="Menu"
		>
			<Icon name={navOpen ? 'x' : 'menu'} size={18} />
		</button>
	</div>

	<aside
		id="sidebar"
		class="{navOpen
			? 'flex'
			: 'hidden'} fixed inset-x-0 top-14 bottom-0 z-20 flex-col bg-surface md:sticky md:top-0 md:flex md:h-dvh md:w-60 md:shrink-0 md:border-r md:border-border"
	>
		<div class="hidden h-14 shrink-0 items-center px-4 md:flex">
			<a href="/servers" aria-label="All servers"><Logo size={26} /></a>
		</div>

		<div class="relative mt-3 px-3 md:mt-1" bind:this={switcher}>
			<button
				onclick={toggleSwitcher}
				aria-expanded={switcherOpen}
				aria-haspopup="menu"
				class="flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-elevated/60"
			>
				{#if guild}
					<GuildIcon name={guild.name} url={guild.icon_url} size={28} />
					<span class="min-w-0 flex-1">
						<span class="block truncate text-sm font-medium">{guild.name}</span>
					</span>
				{:else}
					<span class="size-7 animate-pulse rounded-lg bg-elevated"></span>
					<span class="h-3 flex-1 animate-pulse rounded bg-elevated"></span>
				{/if}
				<Icon name="chevrons-up-down" size={14} class="text-subtle" />
			</button>

			{#if switcherOpen}
				<div
					role="menu"
					class="absolute inset-x-3 top-full z-40 mt-1 max-h-80 overflow-y-auto rounded-lg border border-border-strong bg-elevated p-1 shadow-xl shadow-black/40"
				>
					{#if servers === null}
						<div class="px-3 py-2 text-sm text-muted">Loading servers…</div>
					{:else}
						{#each servers as s (s.id)}
							<a
								role="menuitem"
								href="/servers/{s.id}{section ? '/' + section : ''}"
								class="flex items-center gap-2.5 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-border/60 {s.id ===
								guild?.id
									? 'text-fg'
									: 'text-muted'}"
							>
								<GuildIcon name={s.name} url={s.icon_url} size={22} />
								<span class="min-w-0 flex-1 truncate">{s.name}</span>
								{#if s.id === guild?.id}<Icon name="check" size={14} class="text-accent-ink" />{/if}
							</a>
						{/each}
						<div class="my-1 h-px bg-border"></div>
						<a role="menuitem" href="/servers" class={menuItem}>
							<Icon name="plus" size={14} /> Add or manage servers
						</a>
					{/if}
				</div>
			{/if}
		</div>

		<div class="mt-2 hidden px-3 md:block">
			<button
				onclick={() => (paletteOpen = true)}
				disabled={!guild}
				class="flex h-9 w-full items-center gap-2.5 rounded-lg border border-border bg-bg px-2.5 text-sm text-subtle transition-colors hover:border-border-strong hover:text-muted"
			>
				<Icon name="search" size={15} />
				<span class="flex-1 text-left">Search</span>
				<kbd class="kbd">{shortcut}</kbd>
			</button>
		</div>

		<nav class="mt-5 flex-1 overflow-y-auto px-3" aria-label="Server">
			<ul class="space-y-px">
				{#each primary as item (item.label)}<li>{@render navLink(item)}</li>{/each}
			</ul>
			{#if atLeast(guild?.level, 'admin')}
				<p class="mt-6 mb-1 px-2.5 text-xs text-subtle">Setup</p>
				<ul class="space-y-px">
					{#each setup as item (item.label)}<li>{@render navLink(item)}</li>{/each}
				</ul>
			{/if}
		</nav>

		{#if showReview && plan}
			<div class="mx-3 mb-3 rounded-lg border border-border p-3">
				<p class="text-sm font-medium">Enjoying {APP_NAME}?</p>
				<p class="hint mt-1">
					{guild?.name} has closed {plan.tickets.closed_total.toLocaleString()} tickets with it. A short review helps
					other servers find it.
				</p>
				<div class="mt-3 flex gap-2">
					<a href={plan.review_url} target="_blank" rel="noopener" class="btn btn-secondary h-8 flex-1" onclick={dismissReview}>
						Leave a review
					</a>
					<button class="btn btn-ghost h-8 px-2.5" onclick={dismissReview}>Not now</button>
				</div>
			</div>
		{:else if showUpgrade && plan}
			<div class="mx-3 mb-3 rounded-lg border border-border p-3">
				<p class="text-sm font-medium">Free plan</p>
				<p class="hint mt-1">
					Up to {plan.limits.max_ticket_types} ticket types and {plan.limits.max_panels} ticket panels. Premium
					raises the limits and keeps transcripts forever.
				</p>
				<button class="btn btn-secondary mt-3 h-8 w-full" onclick={upgrade} disabled={checkingOut}>
					{checkingOut ? 'Starting checkout…' : 'Upgrade to premium'}
				</button>
			</div>
		{/if}

		<div class="relative border-t border-border p-3" bind:this={account}>
			{#if accountOpen}
				<div
					role="menu"
					class="absolute inset-x-3 bottom-full z-40 mb-1 rounded-lg border border-border-strong bg-elevated p-1 shadow-xl shadow-black/40"
				>
					<div class="flex items-center justify-between gap-3 px-2.5 py-1.5">
						<span class="text-sm text-muted">Theme</span>
						<div role="radiogroup" aria-label="Theme" class="flex rounded-md border border-border bg-bg p-0.5">
							{#each themeOptions as o (o.value)}
								<button
									type="button"
									role="radio"
									aria-checked={themeState.preference === o.value}
									aria-label={o.label}
									title={o.label}
									onclick={() => setTheme(o.value)}
									class="grid size-7 place-items-center rounded transition-colors {themeState.preference === o.value
										? 'bg-elevated text-fg'
										: 'text-subtle hover:text-fg'}"
								>
									<Icon name={o.icon} size={14} />
								</button>
							{/each}
						</div>
					</div>
					<div class="my-1 h-px bg-border"></div>
					<!-- New tabs, so reading help doesn't lose unsaved changes. -->
					<a role="menuitem" href="/help" target="_blank" class={menuItem}>
						<Icon name="help" size={14} /> Help guides
					</a>
					<a role="menuitem" href={SUPPORT_URL} target="_blank" rel="noopener" class={menuItem}>
						<Icon name="external" size={14} /> Get support
					</a>
					<a role="menuitem" href="/servers" class={menuItem}>
						<Icon name="overview" size={14} /> All servers
					</a>
					<div class="my-1 h-px bg-border"></div>
					<button role="menuitem" class={menuItem} onclick={logout}>
						<Icon name="logout" size={14} /> Log out
					</button>
				</div>
			{/if}
			<button
				onclick={() => (accountOpen = !accountOpen)}
				aria-expanded={accountOpen}
				aria-haspopup="menu"
				class="flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-elevated/60"
			>
				{#if user}
					<img src={user.avatar_url} alt="" class="size-7 rounded-full" />
					<span class="min-w-0 flex-1 truncate text-sm">{user.display_name}</span>
				{:else}
					<span class="size-7 rounded-full bg-elevated"></span>
					<span class="min-w-0 flex-1 truncate text-sm text-muted">Account</span>
				{/if}
				<Icon name="chevrons-up-down" size={14} class="text-subtle" />
			</button>
		</div>
	</aside>

	<main class="min-w-0 flex-1">
		{#if error}
			<div class="mx-auto max-w-md px-5 py-20 text-center">
				<h1 class="font-display text-3xl font-bold">Can't open this server</h1>
				<p class="mt-2 text-sm text-muted">{error}</p>
				<a href="/servers" class="btn btn-secondary mt-6">
					<Icon name="arrow-left" size={14} /> Back to servers
				</a>
			</div>
		{:else if guild && workspace}
			{@render children()}
		{:else}
			<div class="mx-auto max-w-6xl px-5 py-8 md:px-10 md:py-12">
				{#if guild}
					{@render children()}
				{:else}
					<div class="h-9 w-48 animate-pulse rounded-lg bg-elevated"></div>
					<div class="mt-3 h-4 w-80 max-w-full animate-pulse rounded bg-elevated"></div>
				{/if}
			</div>
		{/if}
	</main>
</div>

{#if guild}<CommandPalette bind:open={paletteOpen} {guild} />{/if}
