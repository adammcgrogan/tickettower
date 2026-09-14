<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, errorMessage, send, type SetupProblem, type TicketType } from '$lib/api';
	import { toast } from '$lib/toast.svelte';
	import Dialog from '$lib/components/Dialog.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import SetupProblems from '$lib/components/SetupProblems.svelte';
	import TicketTypeForm from '$lib/components/TicketTypeForm.svelte';

	const guildId = page.params.id!;
	const typeId = Number(page.params.typeId);

	let type = $state<TicketType | null>(null);
	let missing = $state(false);
	let confirmDelete = $state(false);
	let deleting = $state(false);
	let problems = $state<SetupProblem[]>([]);

	function checkSetup() {
		return api<{ problems: SetupProblem[] }>(`/guilds/${guildId}/setup-check`)
			.then((r) => (problems = r.problems.filter((p) => p.id === typeId && p.kind !== 'panel')))
			.catch(() => {});
	}

	onMount(async () => {
		checkSetup();
		try {
			const types = await api<TicketType[]>(`/guilds/${guildId}/ticket-types`);
			type = types.find((t) => t.id === typeId) ?? null;
			missing = !type;
		} catch (e) {
			toast(errorMessage(e), 'error');
		}
	});

	async function remove() {
		deleting = true;
		try {
			const res = await api<{ warning: string }>(
				`/guilds/${guildId}/ticket-types/${typeId}`,
				send('DELETE')
			);
			toast('Ticket type deleted');
			if (res.warning) toast(res.warning, 'info');
			goto(`/servers/${guildId}/ticket-types`);
		} catch (e) {
			toast(errorMessage(e), 'error');
		} finally {
			deleting = false;
			confirmDelete = false;
		}
	}
</script>

<PageHeader
	title={type?.name ?? 'Ticket type'}
	back={{ href: `/servers/${guildId}/ticket-types`, label: 'Ticket types' }}
>
	{#snippet actions()}
		{#if type}
			<a href="/servers/{guildId}/ticket-types/new?from={type.id}" class="btn btn-secondary">
				<Icon name="copy" size={15} /> Duplicate
			</a>
			<button class="btn btn-ghost text-danger hover:text-danger" onclick={() => (confirmDelete = true)}>
				<Icon name="trash" size={15} /> Delete
			</button>
		{/if}
	{/snippet}
</PageHeader>

<div class="mt-6">
	{#if missing}
		<p class="card p-5 text-sm text-muted">This ticket type doesn't exist any more.</p>
	{:else if type}
		{#if problems.length}
			<div class="mb-6">
				<SetupProblems {problems} {guildId} onrecheck={checkSetup} />
			</div>
		{/if}
		<TicketTypeForm {guildId} initial={type} onsaved={(name) => type && (type.name = name)} />
	{:else}
		<div class="card h-64 animate-pulse"></div>
	{/if}
</div>

<Dialog
	bind:open={confirmDelete}
	title="Delete this ticket type?"
	description="Its button will be removed from any ticket panel messages. Closed tickets keep their history. A type with open tickets can't be deleted: close them or move them to another type first."
>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (confirmDelete = false)}>Cancel</button>
		<button class="btn btn-danger" onclick={remove} disabled={deleting}>
			{deleting ? 'Deleting…' : 'Delete ticket type'}
		</button>
	{/snippet}
</Dialog>
