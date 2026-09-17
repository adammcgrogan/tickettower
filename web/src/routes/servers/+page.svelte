<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Guild } from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import AppHeader from '$lib/components/AppHeader.svelte';
	import GuildIcon from '$lib/components/GuildIcon.svelte';
	import Icon from '$lib/components/Icon.svelte';

	let guilds = $state<Guild[] | null>(null);
	let error = $state('');
	let query = $state('');

	// Servers with the bot first, then alphabetical.
	const filtered = $derived(
		(guilds ?? [])
			.filter((g) => g.name.toLowerCase().includes(query.trim().toLowerCase()))
			.sort((a, b) => Number(b.bot_present) - Number(a.bot_present) || a.name.localeCompare(b.name))
	);

	async function load() {
		try {
			guilds = await api<Guild[]>('/guilds');
			error = '';
		} catch (e) {
			error = (e as Error).message;
		}
	}

	onMount(load);
</script>

<svelte:head><title>Servers · {APP_NAME}</title></svelte:head>

<!-- Refresh when returning from the Discord invite tab. -->
<svelte:window onfocus={load} />

<AppHeader />

<main class="mx-auto max-w-6xl px-5 py-12">
	<div class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
		<div>
			<h1 class="font-display text-4xl leading-none font-bold">Your servers</h1>
			<p class="mt-2.5 text-sm text-muted">
				Pick a server to manage, or add {APP_NAME} to one you run.
			</p>
		</div>
		<label class="relative block sm:w-64">
			<span class="sr-only">Search servers</span>
			<Icon
				name="search"
				size={15}
				class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle"
			/>
			<input
				bind:value={query}
				placeholder="Search servers"
				class="h-9 w-full rounded-lg border border-border bg-surface pr-3 pl-9 text-sm placeholder:text-subtle focus:border-accent focus:ring-2 focus:ring-accent/25 focus:outline-none"
			/>
		</label>
	</div>

	{#if error && !guilds}
		<div class="mt-8 rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm">
			<p class="text-danger">{error}</p>
			<button onclick={load} class="mt-3 text-muted underline-offset-4 hover:text-fg hover:underline">
				Try again
			</button>
		</div>
	{:else if guilds === null}
		<ul class="mt-8 grid gap-3 sm:grid-cols-2 lg:grid-cols-3" aria-busy="true">
			{#each Array(6) as _, i (i)}
				<li class="flex h-[78px] items-center gap-4 rounded-xl border border-border bg-surface p-4">
					<div class="size-11 animate-pulse rounded-xl bg-elevated"></div>
					<div class="flex-1 space-y-2">
						<div class="h-3 w-2/3 animate-pulse rounded bg-elevated"></div>
						<div class="h-2.5 w-1/3 animate-pulse rounded bg-elevated"></div>
					</div>
				</li>
			{/each}
		</ul>
	{:else if filtered.length === 0}
		<div class="mt-8 rounded-xl border border-dashed border-border px-6 py-16 text-center">
			<p class="font-medium">{query ? 'No servers match your search' : 'No servers to manage'}</p>
			<p class="mx-auto mt-1 max-w-sm text-sm text-muted">
				{query
					? 'Try a different name.'
					: 'You need the Manage Server permission, or a dashboard role, in a server to manage it.'}
			</p>
		</div>
	{:else}
		<ul class="mt-8 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each filtered as g (g.id)}
				<li>
					{#if g.bot_present}
						<a
							href="/servers/{g.id}"
							class="group flex items-center gap-4 rounded-xl border border-border bg-surface p-4 transition-colors hover:border-border-strong hover:bg-elevated"
						>
							<GuildIcon name={g.name} url={g.icon_url} />
							<div class="min-w-0 flex-1">
								<div class="truncate font-medium">{g.name}</div>
								<div class="mt-0.5 flex items-center gap-1.5 text-xs text-muted">
									<span class="size-1.5 rounded-full bg-success"></span>
									Active
								</div>
							</div>
							<Icon
								name="arrow-right"
								class="text-subtle transition-all group-hover:translate-x-0.5 group-hover:text-fg"
							/>
						</a>
					{:else}
						<a
							href="/api/invite?guild_id={g.id}&ref=dashboard"
							target="_blank"
							rel="noopener"
							class="group flex items-center gap-4 rounded-xl border border-border bg-surface/50 p-4 transition-colors hover:border-border-strong hover:bg-elevated"
						>
							<div class="opacity-60 transition-opacity group-hover:opacity-100">
								<GuildIcon name={g.name} url={g.icon_url} />
							</div>
							<div class="min-w-0 flex-1">
								<div class="truncate font-medium text-muted group-hover:text-fg">{g.name}</div>
								<div class="mt-0.5 text-xs text-subtle">Not set up</div>
							</div>
							<span
								class="rounded-md border border-border-strong px-2.5 py-1 text-xs font-medium text-muted transition-colors group-hover:border-accent group-hover:bg-accent group-hover:text-on-accent"
							>
								Set up
							</span>
						</a>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</main>
