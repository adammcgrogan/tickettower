<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type User } from '$lib/api';
	import Icon from './Icon.svelte';
	import Logo from './Logo.svelte';

	let user = $state<User | null>(null);
	let open = $state(false);
	let menu = $state<HTMLElement>();

	onMount(async () => {
		user = await api<User>('/me');
	});

	async function logout() {
		await api('/auth/logout', { method: 'POST' });
		window.location.href = '/';
	}
</script>

<svelte:window
	onclick={(e) => {
		if (open && menu && !menu.contains(e.target as Node)) open = false;
	}}
	onkeydown={(e) => {
		if (e.key === 'Escape') open = false;
	}}
/>

<header class="sticky top-0 z-20 border-b border-border bg-bg/80 backdrop-blur-md">
	<div class="mx-auto flex h-14 max-w-6xl items-center justify-between px-5">
		<a href="/servers" aria-label="Your servers"><Logo /></a>

		{#if user}
			<div class="relative" bind:this={menu}>
				<button
					onclick={() => (open = !open)}
					aria-expanded={open}
					aria-haspopup="menu"
					class="flex items-center gap-2 rounded-lg py-1.5 pr-2 pl-1.5 text-sm transition-colors hover:bg-elevated"
				>
					<img src={user.avatar_url} alt="" class="size-6 rounded-full" />
					<span class="hidden sm:inline">{user.display_name}</span>
					<Icon name="chevron-down" size={14} class="text-muted" />
				</button>

				{#if open}
					<div
						role="menu"
						class="absolute right-0 mt-1.5 w-52 rounded-xl border border-border bg-surface p-1 shadow-2xl shadow-black/50"
					>
						<div class="px-3 pt-2 pb-2.5">
							<div class="truncate text-sm font-medium">{user.display_name}</div>
							<div class="truncate text-xs text-muted">@{user.username}</div>
						</div>
						<div class="my-1 h-px bg-border"></div>
						{#if user.is_superadmin}
							<a
								role="menuitem"
								href="/admin"
								class="block w-full rounded-lg px-3 py-2 text-left text-sm text-muted transition-colors hover:bg-elevated hover:text-fg"
							>
								Admin
							</a>
						{/if}
						<button
							role="menuitem"
							onclick={logout}
							class="w-full rounded-lg px-3 py-2 text-left text-sm text-muted transition-colors hover:bg-elevated hover:text-fg"
						>
							Log out
						</button>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</header>
