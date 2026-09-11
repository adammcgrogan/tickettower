<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, errorMessage, type Channel, type TicketType } from '$lib/api';
	import { emojiText } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';

	const guildId = page.params.id!;

	let types = $state<TicketType[] | null>(null);
	let channels = $state<Channel[]>([]);
	let error = $state('');

	onMount(async () => {
		try {
			types = await api<TicketType[]>(`/guilds/${guildId}/ticket-types`);
			channels = await api<Channel[]>(`/guilds/${guildId}/channels`).catch(() => []);
		} catch (e) {
			error = errorMessage(e);
		}
	});

	function where(t: TicketType) {
		const parent = channels.find((c) => c.id === t.parent_id);
		if (t.mode === 'thread') return parent ? `Private threads in #${parent.name}` : 'Private threads';
		return parent ? `Private channels in ${parent.name}` : 'Private channels';
	}

	const roleCount = (n: number) =>
		n === 0 ? 'Managers only' : `${n} support role${n === 1 ? '' : 's'}`;
</script>

<PageHeader
	title="Ticket types"
	description="The kinds of tickets members can open. Each type decides where tickets go and who handles them."
>
	{#snippet actions()}
		<a href="/servers/{guildId}/ticket-types/new" class="btn btn-primary">
			<Icon name="plus" size={15} /> New ticket type
		</a>
	{/snippet}
</PageHeader>

<div class="mt-6">
	{#if error}
		<p class="card p-5 text-sm text-danger">{error}</p>
	{:else if types === null}
		<div class="space-y-2">
			{#each Array(3) as _, i (i)}
				<div class="card h-[68px] animate-pulse"></div>
			{/each}
		</div>
	{:else if types.length === 0}
		<div class="rounded-xl border border-dashed border-border px-6 py-14 text-center">
			<div class="mx-auto grid size-10 place-items-center rounded-xl bg-elevated text-muted">
				<Icon name="tag" />
			</div>
			<p class="mt-4 font-medium">No ticket types yet</p>
			<p class="mx-auto mt-1 max-w-sm text-sm text-muted">
				Create one for each kind of help you offer, like "General support" or "Billing".
			</p>
			<a href="/servers/{guildId}/ticket-types/new" class="btn btn-primary mt-5">
				<Icon name="plus" size={15} /> Create ticket type
			</a>
		</div>
	{:else}
		<ul class="card divide-y divide-border overflow-hidden">
			{#each types as t (t.id)}
				<li>
					<a
						href="/servers/{guildId}/ticket-types/{t.id}"
						class="group flex items-center gap-4 px-4 py-3.5 transition-colors hover:bg-elevated"
					>
						<span class="grid size-9 shrink-0 place-items-center rounded-lg bg-elevated text-base">
							{#if t.emoji}{emojiText(t.emoji)}{:else}<Icon name="tag" class="text-muted" />{/if}
						</span>
						<div class="min-w-0 flex-1">
							<div class="truncate text-sm font-medium">{t.name}</div>
							<div class="mt-0.5 truncate text-xs text-muted">
								{where(t)} · {roleCount(t.support_role_ids.length)}
							</div>
						</div>
						<Icon
							name="arrow-right"
							class="text-subtle transition-all group-hover:translate-x-0.5 group-hover:text-fg"
						/>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</div>
