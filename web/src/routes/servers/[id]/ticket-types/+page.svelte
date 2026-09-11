<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, errorMessage, type Channel, type TicketType } from '$lib/api';
	import { emojiText, hoursLabel } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';

	const guildId = page.params.id!;

	let types = $state<TicketType[] | null>(null);
	let channels = $state<Channel[]>([]);
	let error = $state('');

	async function load() {
		error = '';
		try {
			types = await api<TicketType[]>(`/guilds/${guildId}/ticket-types`);
			channels = await api<Channel[]>(`/guilds/${guildId}/channels`).catch(() => []);
		} catch (e) {
			error = errorMessage(e);
		}
	}
	onMount(load);

	function where(t: TicketType) {
		const parent = channels.find((c) => c.id === t.parent_id);
		if (t.mode === 'thread') return parent ? `Private threads in #${parent.name}` : 'Private threads';
		return parent ? `Private channels in ${parent.name}` : 'Private channels';
	}

	function facts(t: TicketType) {
		const n = t.support_role_ids.length;
		const out = [n === 0 ? 'Managers only' : `${n} support role${n === 1 ? '' : 's'}`];
		if (t.questions.length) out.push(`${t.questions.length} question${t.questions.length === 1 ? '' : 's'}`);
		if (t.auto_close_hours) out.push(`Auto-closes after ${hoursLabel(t.auto_close_hours)}`);
		return out;
	}
</script>

<PageHeader
	title="Ticket types"
	description="The kinds of help members can ask for. Each type decides where its tickets open and who handles them."
>
	{#snippet actions()}
		{#if types?.length}
			<a href="/servers/{guildId}/ticket-types/new" class="btn btn-primary">
				<Icon name="plus" size={15} /> New ticket type
			</a>
		{/if}
	{/snippet}
</PageHeader>

<div class="mt-8">
	{#if error}
		<div class="rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm">
			<p class="text-danger">{error}</p>
			<button onclick={load} class="mt-3 text-muted underline-offset-4 hover:text-fg hover:underline">
				Try again
			</button>
		</div>
	{:else if types === null}
		<div class="space-y-px overflow-hidden rounded-xl border border-border" aria-busy="true">
			{#each Array(3) as _, i (i)}<div class="h-[76px] animate-pulse bg-surface"></div>{/each}
		</div>
	{:else if types.length === 0}
		<div class="rounded-xl border border-dashed border-border px-6 py-14 text-center">
			<p class="font-medium">No ticket types yet</p>
			<p class="mx-auto mt-1 max-w-sm text-sm text-muted">
				Create one for each kind of help you offer, like "General support" or "Billing".
			</p>
			<a href="/servers/{guildId}/ticket-types/new" class="btn btn-primary mt-5">
				<Icon name="plus" size={15} /> Create a ticket type
			</a>
		</div>
	{:else}
		<ul class="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
			{#each types as t (t.id)}
				<li>
					<a
						href="/servers/{guildId}/ticket-types/{t.id}"
						class="group flex items-center gap-4 px-4 py-4 transition-colors hover:bg-elevated/60"
					>
						<span class="grid size-10 shrink-0 place-items-center rounded-lg bg-elevated text-lg">
							{#if t.emoji}{emojiText(t.emoji)}{:else}<Icon name="tag" class="text-muted" />{/if}
						</span>
						<div class="min-w-0 flex-1">
							<div class="truncate font-medium">{t.name}</div>
							<div class="mt-0.5 truncate text-sm text-muted">{where(t)}</div>
						</div>
						<ul class="hidden flex-wrap justify-end gap-1.5 md:flex">
							{#each facts(t) as fact (fact)}
								<li class="rounded-md border border-border px-2 py-0.5 text-xs text-muted">{fact}</li>
							{/each}
						</ul>
						<Icon name="arrow-right" class="text-subtle transition-colors group-hover:text-fg" />
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</div>
