<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, errorMessage, send, type Channel, type Panel, type TicketType } from '$lib/api';
	import { toast } from '$lib/toast.svelte';
	import Dialog from '$lib/components/Dialog.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import PanelEditor from '$lib/components/PanelEditor.svelte';

	const guildId = page.params.id!;
	const panelId = Number(page.params.panelId);

	let data = $state<{ panel: Panel; types: TicketType[]; channels: Channel[] } | null>(null);
	let missing = $state(false);
	let confirmDelete = $state(false);
	let deleting = $state(false);

	onMount(async () => {
		try {
			const [panels, types, channels] = await Promise.all([
				api<Panel[]>(`/guilds/${guildId}/panels`),
				api<TicketType[]>(`/guilds/${guildId}/ticket-types`),
				api<Channel[]>(`/guilds/${guildId}/channels`)
			]);
			const panel = panels.find((p) => p.id === panelId);
			if (panel) data = { panel, types, channels };
			else missing = true;
		} catch (e) {
			toast(errorMessage(e), 'error');
		}
	});

	async function remove() {
		deleting = true;
		try {
			await api(`/guilds/${guildId}/panels/${panelId}`, send('DELETE'));
			toast('Ticket buttons deleted');
			goto(`/servers/${guildId}/buttons`);
		} catch (e) {
			toast(errorMessage(e), 'error');
		} finally {
			deleting = false;
			confirmDelete = false;
		}
	}
</script>

<PageHeader
	title={data?.panel.title ?? 'Ticket buttons'}
	back={{ href: `/servers/${guildId}/buttons`, label: 'Ticket buttons' }}
>
	{#snippet actions()}
		{#if data}
			<button class="btn btn-ghost text-danger hover:text-danger" onclick={() => (confirmDelete = true)}>
				<Icon name="trash" size={15} /> Delete
			</button>
		{/if}
	{/snippet}
</PageHeader>

<div class="mt-6">
	{#if missing}
		<p class="card p-5 text-sm text-muted">These ticket buttons don't exist any more.</p>
	{:else if data}
		<PanelEditor {guildId} initial={data.panel} types={data.types} channels={data.channels} />
	{:else}
		<div class="card h-96 animate-pulse"></div>
	{/if}
</div>

<Dialog
	bind:open={confirmDelete}
	title="Delete these ticket buttons?"
	description="If they're published, the message will be removed from Discord too. Open tickets aren't affected."
>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (confirmDelete = false)}>Cancel</button>
		<button class="btn btn-danger" onclick={remove} disabled={deleting}>
			{deleting ? 'Deleting…' : 'Delete ticket buttons'}
		</button>
	{/snippet}
</Dialog>
