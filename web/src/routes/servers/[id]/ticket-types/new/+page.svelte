<script lang="ts">
	import { page } from '$app/state';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import TicketTypeForm from '$lib/components/TicketTypeForm.svelte';

	const guildId = page.params.id!;
</script>

<PageHeader
	title="New ticket type"
	description="Basics is all you need to start. The other tabs have sensible defaults you can change any time."
	back={{ href: `/servers/${guildId}/ticket-types`, label: 'Ticket types' }}
/>

<div class="mt-6">
	<TicketTypeForm {guildId} />
</div>
