<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, errorMessage, type Channel, type TicketType } from '$lib/api';
	import { toast } from '$lib/toast.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import PanelEditor from '$lib/components/PanelEditor.svelte';

	const guildId = page.params.id!;

	let data = $state<{ types: TicketType[]; channels: Channel[] } | null>(null);

	onMount(async () => {
		try {
			const [types, channels] = await Promise.all([
				api<TicketType[]>(`/guilds/${guildId}/ticket-types`),
				api<Channel[]>(`/guilds/${guildId}/channels`)
			]);
			data = { types, channels };
		} catch (e) {
			toast(errorMessage(e), 'error');
		}
	});
</script>

<PageHeader
	title="New ticket buttons"
	description="Design the message members will click to open tickets. The preview updates as you type."
	back={{ href: `/servers/${guildId}/buttons`, label: 'Ticket buttons' }}
/>

<div class="mt-6">
	{#if data}
		<PanelEditor {guildId} types={data.types} channels={data.channels} />
	{:else}
		<div class="card h-96 animate-pulse"></div>
	{/if}
</div>
