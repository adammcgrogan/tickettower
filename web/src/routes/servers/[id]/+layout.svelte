<script lang="ts">
	import { setContext } from 'svelte';
	import { page } from '$app/state';
	import { api, type Guild } from '$lib/api';
	import GuildIcon from '$lib/components/GuildIcon.svelte';
	import Icon, { type IconName } from '$lib/components/Icon.svelte';

	let { children } = $props();

	let guild = $state<Guild | null>(null);
	let error = $state('');

	setContext('guild', () => guild);

	$effect(() => {
		const id = page.params.id;
		guild = null;
		error = '';
		api<Guild>(`/guilds/${id}`)
			.then((g) => (guild = g))
			.catch((e: Error) => (error = e.message));
	});

	const base = $derived(`/servers/${page.params.id}`);

	type NavItem = { label: string; href: string; icon: IconName; ready: boolean };
	const nav: NavItem[] = $derived([
		{ label: 'Overview', href: base, icon: 'overview', ready: true },
		{ label: 'Ticket types', href: `${base}/ticket-types`, icon: 'tag', ready: true },
		{ label: 'Panels', href: `${base}/panels`, icon: 'panel', ready: true },
		{ label: 'Tickets', href: `${base}/tickets`, icon: 'inbox', ready: true },
		{ label: 'Analytics', href: `${base}/analytics`, icon: 'chart', ready: true },
		{ label: 'Settings', href: `${base}/settings`, icon: 'settings', ready: true }
	]);
</script>

{#if error}
	<div class="mx-auto max-w-md px-5 py-24 text-center">
		<h1 class="text-lg font-semibold">Can't open this server</h1>
		<p class="mt-1 text-sm text-muted">{error}</p>
		<a
			href="/servers"
			class="mt-6 inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-2 text-sm transition-colors hover:bg-elevated"
		>
			<Icon name="arrow-left" size={14} /> Back to servers
		</a>
	</div>
{:else}
	<div class="mx-auto flex max-w-6xl flex-col gap-8 px-5 py-8 md:flex-row">
		<aside class="md:w-56 md:shrink-0">
			<a
				href="/servers"
				class="mb-4 inline-flex items-center gap-1.5 text-xs text-muted transition-colors hover:text-fg"
			>
				<Icon name="arrow-left" size={13} /> All servers
			</a>

			<div class="flex h-[62px] items-center gap-3 rounded-xl border border-border bg-surface p-3">
				{#if guild}
					<GuildIcon name={guild.name} url={guild.icon_url} size={36} />
					<div class="min-w-0 truncate text-sm font-medium">{guild.name}</div>
				{:else}
					<div class="size-9 animate-pulse rounded-xl bg-elevated"></div>
					<div class="h-3 w-24 animate-pulse rounded bg-elevated"></div>
				{/if}
			</div>

			<nav class="-mx-1 mt-4 flex gap-0.5 overflow-x-auto px-1 md:mx-0 md:flex-col md:px-0">
				{#each nav as item (item.label)}
					{@const active =
						page.url.pathname === item.href ||
						(item.href !== base && page.url.pathname.startsWith(item.href + '/'))}
					{#if item.ready}
						<a
							href={item.href}
							aria-current={active ? 'page' : undefined}
							class="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm whitespace-nowrap transition-colors {active
								? 'bg-elevated text-fg'
								: 'text-muted hover:bg-elevated/60 hover:text-fg'}"
						>
							<Icon name={item.icon} />
							{item.label}
						</a>
					{:else}
						<span
							class="flex cursor-default items-center gap-2.5 rounded-lg px-3 py-2 text-sm whitespace-nowrap text-subtle"
							title="Coming soon"
						>
							<Icon name={item.icon} />
							{item.label}
							<span
								class="ml-auto rounded bg-elevated px-1.5 py-0.5 text-[10px] font-medium tracking-wide uppercase"
							>
								Soon
							</span>
						</span>
					{/if}
				{/each}
			</nav>
		</aside>

		<main class="min-w-0 flex-1">
			{#if guild}
				{@render children()}
			{/if}
		</main>
	</div>
{/if}
