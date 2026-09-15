<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, errorMessage, type Channel, type Panel, type SetupProblem, type TicketType } from '$lib/api';
	import { emojiText, hoursLabel } from '$lib/format';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import LoadError from '$lib/components/LoadError.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';

	const guildId = page.params.id!;

	let types = $state<TicketType[] | null>(null);
	let channels = $state<Channel[]>([]);
	let panels = $state<Panel[]>([]);
	let problems = $state<SetupProblem[]>([]);
	let error = $state('');

	// Ticket types that members can't open, or can't choose.
	const needsAttention = $derived(
		new Set(problems.filter((p) => p.kind === 'ticket_type' || p.kind === 'unlisted').map((p) => p.id))
	);

	async function load() {
		error = '';
		api<{ problems: SetupProblem[] }>(`/guilds/${guildId}/setup-check`)
			.then((r) => (problems = r.problems))
			.catch(() => {});
		try {
			[types, panels] = await Promise.all([
				api<TicketType[]>(`/guilds/${guildId}/ticket-types`),
				api<Panel[]>(`/guilds/${guildId}/panels`).catch(() => [])
			]);
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

	// Which ticket panels show the type, since that's where members open it.
	function shownOn(t: TicketType) {
		const on = panels.filter((p) => p.ticket_type_ids.includes(t.id));
		if (on.length === 0) return 'Not on any ticket panel';
		return on.length === 1 ? `On “${on[0].title}”` : `On ${on.length} ticket panels`;
	}

	function facts(t: TicketType) {
		const n = t.support_role_ids.length;
		const out = [n === 0 ? 'Managers only' : `${n} support role${n === 1 ? '' : 's'}`];
		if (t.questions.length) out.push(`${t.questions.length} question${t.questions.length === 1 ? '' : 's'}`);
		if (t.auto_close_hours) out.push(`Auto-closes after ${hoursLabel(t.auto_close_hours)}`);
		if (t.required_role_ids.length) out.push('Some roles only');
		if (t.auto_assign) out.push('Auto-assigns');
		if (!t.ask_rating) out.push('No rating request');
		return out;
	}

	// Rows show a couple of facts; the rest are behind "+N" so the list stays scannable.
	const SHOWN_FACTS = 2;
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

<div class="mt-8 max-w-4xl">
	{#if error}
		<LoadError message={error} onretry={load} />
	{:else if types === null}
		<div class="space-y-px overflow-hidden rounded-xl border border-border" aria-busy="true">
			{#each Array(3) as _, i (i)}<div class="h-[76px] animate-pulse bg-surface"></div>{/each}
		</div>
	{:else if types.length === 0}
		<EmptyState icon="tag" title="No ticket types yet">
			A ticket type decides where its tickets open, who handles them, and what members are asked first.
			{#snippet action()}
				<a href="/servers/{guildId}/ticket-types/new" class="btn btn-primary">
					<Icon name="plus" size={15} /> New ticket type
				</a>
			{/snippet}
		</EmptyState>
	{:else}
		<ul class="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
			{#each types as t (t.id)}
				{@const all = facts(t)}
				<li class="flex items-center">
					<a
						href="/servers/{guildId}/ticket-types/{t.id}"
						class="group flex min-w-0 flex-1 items-center gap-4 px-4 py-4 transition-colors hover:bg-elevated/60"
					>
						<span class="grid size-10 shrink-0 place-items-center rounded-lg bg-elevated text-lg">
							{#if t.emoji}{emojiText(t.emoji)}{:else}<Icon name="tag" class="text-muted" />{/if}
						</span>
						<div class="min-w-0 flex-1">
							<div class="truncate font-medium">{t.name}</div>
							{#if needsAttention.has(t.id)}
								<div class="mt-0.5 flex items-center gap-1.5 text-sm text-danger">
									<Icon name="alert" size={14} /> Needs attention
								</div>
							{:else}
								<div class="mt-0.5 truncate text-sm text-muted">{where(t)}</div>
							{/if}
						</div>
						<ul class="hidden flex-wrap justify-end gap-1.5 md:flex">
							<li
								class="rounded-md border px-2 py-0.5 text-xs {panels.some((p) => p.ticket_type_ids.includes(t.id))
									? 'border-border text-muted'
									: 'border-dashed border-border-strong text-subtle'}"
							>
								{shownOn(t)}
							</li>
							{#each all.slice(0, SHOWN_FACTS) as fact (fact)}
								<li class="rounded-md border border-border px-2 py-0.5 text-xs text-muted">{fact}</li>
							{/each}
							{#if all.length > SHOWN_FACTS}
								{@const rest = all.slice(SHOWN_FACTS)}
								<li class="rounded-md border border-border px-2 py-0.5 text-xs text-muted" title={rest.join('\n')}>
									<span aria-hidden="true">+{rest.length}</span>
									<span class="sr-only">{rest.join(', ')}</span>
								</li>
							{/if}
						</ul>
						<Icon name="arrow-right" class="text-subtle transition-colors group-hover:text-fg" />
					</a>
					<a
						href="/servers/{guildId}/ticket-types/new?from={t.id}"
						class="btn btn-ghost mr-2 size-8 shrink-0 p-0"
						aria-label="Duplicate {t.name}"
						title="Duplicate"
					>
						<Icon name="copy" size={15} />
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</div>
