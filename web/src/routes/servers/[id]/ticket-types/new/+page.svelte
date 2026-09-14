<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, errorMessage, type TicketType } from '$lib/api';
	import { toast } from '$lib/toast.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import TicketTypeForm from '$lib/components/TicketTypeForm.svelte';

	const guildId = page.params.id!;
	// ?from=<id> duplicates that type: the form starts with a copy of its settings.
	const fromId = Number(page.url.searchParams.get('from')) || null;

	let copyOf = $state<TicketType | null>(null);
	let ready = $state(!fromId);

	onMount(async () => {
		if (!fromId) return;
		try {
			const types = await api<TicketType[]>(`/guilds/${guildId}/ticket-types`);
			copyOf = types.find((t) => t.id === fromId) ?? null;
			if (!copyOf) toast("That ticket type doesn't exist any more, so this one starts blank.", 'info');
		} catch (e) {
			toast(errorMessage(e), 'error');
		} finally {
			ready = true;
		}
	});
</script>

<PageHeader
	title={copyOf ? `Copy of ${copyOf.name}` : 'New ticket type'}
	description={copyOf
		? 'Every setting is copied over. Change what you need, then create it. Nothing is saved until you do.'
		: 'Basics is all you need to start. The other tabs have sensible defaults you can change any time.'}
	back={{ href: `/servers/${guildId}/ticket-types`, label: 'Ticket types' }}
/>

<div class="mt-6">
	{#if ready}
		<TicketTypeForm {guildId} copyOf={copyOf ?? undefined} />
	{:else}
		<div class="card h-64 animate-pulse" aria-busy="true"></div>
	{/if}
</div>
