<script lang="ts">
	import { page } from '$app/state';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import TicketTypeForm from '$lib/components/TicketTypeForm.svelte';

	const guildId = page.params.id!;
</script>

<PageHeader
	title="New ticket type"
	description="Decide where these tickets open and who handles them. You can change this later."
	back={{ href: `/servers/${guildId}/ticket-types`, label: 'Ticket types' }}
/>

<div class="mt-6">
	<TicketTypeForm {guildId} />
</div>
