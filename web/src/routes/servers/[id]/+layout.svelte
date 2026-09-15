<script lang="ts">
	import { onMount, setContext } from 'svelte';
	import { page } from '$app/state';
	import { api, atLeast, errorMessage, send, type Guild, type Stats, type Ticket, type User } from '$lib/api';
	import { SUPPORT_URL } from '$lib/brand';
	import { toast } from '$lib/toast.svelte';
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
	let servers = $state<Guild[] | null>(null);
	let switcher = $state<HTMLElement>();

	setContext('guild', () => guild);

	$effect(() => {
		const id = page.params.id;
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
	});

	onMount(async () => {
		user = await api<User>('/me').catch(() => null);
	});

	const base = $derived(`/servers/${page.params.id}`);
	// The current section, kept when switching servers.
	const section = $derived(page.url.pathname.split('/')[3] ?? '');

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

	async function logout() {
		await api('/auth/logout', { method: 'POST' });
		window.location.href = '/';
	}
</script>

<svelte:window
	onclick={(e) => {
		if (switcherOpen && switcher && !switcher.contains(e.target as Node)) switcherOpen = false;
	}}
	onkeydown={(e) => {
		if (e.key === 'Escape') switcherOpen = navOpen = false;
	}}
/>

{#snippet navLink(item: NavItem)}
	{@const active = isActive(item.href)}
	<a
		href={item.href}
		aria-current={active ? 'page' : undefined}
		class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors {active
			? 'bg-elevated font-medium text-fg'
			: 'text-muted hover:bg-elevated/60 hover:text-fg'}"
	>
		<Icon name={item.icon} class={active ? 'text-accent' : ''} />
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
		class="sticky top-0 z-30 flex h-14 items-center justify-between border-b border-border bg-bg/90 px-4 backdrop-blur md:hidden"
	>
		<a href="/servers" aria-label="All servers"><Logo size={26} /></a>
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
		<div class="hidden h-16 shrink-0 items-center px-5 md:flex">
			<a href="/servers" aria-label="All servers"><Logo /></a>
		</div>

		<div class="relative mt-3 px-3 md:mt-0" bind:this={switcher}>
			<button
				onclick={toggleSwitcher}
				aria-expanded={switcherOpen}
				aria-haspopup="menu"
				class="flex w-full items-center gap-2.5 rounded-lg border border-border bg-bg px-2.5 py-2 text-left transition-colors hover:border-border-strong"
			>
				{#if guild}
					<GuildIcon name={guild.name} url={guild.icon_url} size={28} />
					<span class="min-w-0 flex-1 truncate text-sm font-medium">{guild.name}</span>
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
								{#if s.id === guild?.id}<Icon name="check" size={14} class="text-accent" />{/if}
							</a>
						{/each}
						<div class="my-1 h-px bg-border"></div>
						<a
							role="menuitem"
							href="/servers"
							class="block rounded-md px-2 py-1.5 text-sm text-muted transition-colors hover:bg-border/60 hover:text-fg"
						>
							All servers
						</a>
					{/if}
				</div>
			{/if}
		</div>

		<nav class="mt-5 flex-1 overflow-y-auto px-3" aria-label="Server">
			<ul class="space-y-0.5">
				{#each primary as item (item.label)}<li>{@render navLink(item)}</li>{/each}
			</ul>
			{#if atLeast(guild?.level, 'admin')}
				<p class="mt-7 mb-1.5 px-3 text-xs font-medium text-subtle">Setup</p>
				<ul class="space-y-0.5">
					{#each setup as item (item.label)}<li>{@render navLink(item)}</li>{/each}
				</ul>
			{/if}
		</nav>

		{#if showUpgrade && plan}
			<div class="mx-3 mb-3 rounded-lg border border-border p-3">
				<p class="flex items-center gap-2 text-sm font-medium">
					<Icon name="sparkles" size={15} class="text-muted" /> Free plan
				</p>
				<p class="hint mt-1">
					Up to {plan.limits.max_ticket_types} ticket types and {plan.limits.max_panels} ticket panels. Premium
					raises the limits and keeps transcripts forever.
				</p>
				<button class="btn btn-secondary mt-3 h-8 w-full" onclick={upgrade} disabled={checkingOut}>
					{checkingOut ? 'Starting checkout…' : 'Upgrade to premium'}
				</button>
			</div>
		{/if}

		<!-- New tabs, so reading help doesn't lose unsaved changes. -->
		<ul class="space-y-0.5 px-3 pb-3">
			<li>
				<a
					href="/help"
					target="_blank"
					class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm text-muted transition-colors hover:bg-elevated/60 hover:text-fg"
				>
					<Icon name="help" />
					<span class="flex-1">Help</span>
				</a>
			</li>
			<li>
				<a
					href={SUPPORT_URL}
					target="_blank"
					rel="noopener"
					class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm text-muted transition-colors hover:bg-elevated/60 hover:text-fg"
				>
					<Icon name="external" />
					<span class="flex-1">Get support</span>
				</a>
			</li>
		</ul>

		{#if user}
			<div class="flex items-center gap-2.5 border-t border-border p-3">
				<img src={user.avatar_url} alt="" class="size-7 rounded-full" />
				<span class="min-w-0 flex-1 truncate text-sm">{user.display_name}</span>
				<button onclick={logout} class="btn btn-ghost size-8 p-0" aria-label="Log out" title="Log out">
					<Icon name="logout" size={15} />
				</button>
			</div>
		{/if}
	</aside>

	<main class="min-w-0 flex-1">
		<div class="mx-auto max-w-6xl px-5 py-8 md:px-10 md:py-12">
			{#if error}
				<div class="mx-auto max-w-md py-20 text-center">
					<h1 class="font-display text-3xl font-bold">Can't open this server</h1>
					<p class="mt-2 text-sm text-muted">{error}</p>
					<a href="/servers" class="btn btn-secondary mt-6">
						<Icon name="arrow-left" size={14} /> Back to servers
					</a>
				</div>
			{:else if guild}
				{@render children()}
			{:else}
				<div class="h-9 w-48 animate-pulse rounded-lg bg-elevated"></div>
				<div class="mt-3 h-4 w-80 max-w-full animate-pulse rounded bg-elevated"></div>
			{/if}
		</div>
	</main>
</div>
