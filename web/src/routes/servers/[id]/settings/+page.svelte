<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, errorMessage, send } from '$lib/api';
	import { toast } from '$lib/toast.svelte';
	import Field from '$lib/components/Field.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';

	type Settings = { transcript_retention_days: number | null };

	const guildId = page.params.id!;
	const retention = [
		{ value: '', label: 'Keep forever' },
		{ value: '7', label: '7 days' },
		{ value: '30', label: '30 days' },
		{ value: '90', label: '90 days' },
		{ value: '180', label: '6 months' },
		{ value: '365', label: '1 year' }
	];

	let settings = $state<Settings | null>(null);
	let retentionValue = $state('');
	let saving = $state(false);

	const dirty = $derived(
		settings !== null && retentionValue !== (settings.transcript_retention_days?.toString() ?? '')
	);

	onMount(async () => {
		try {
			settings = await api<Settings>(`/guilds/${guildId}/settings`);
			retentionValue = settings.transcript_retention_days?.toString() ?? '';
		} catch (e) {
			toast(errorMessage(e), 'error');
		}
	});

	async function save(e: SubmitEvent) {
		e.preventDefault();
		saving = true;
		try {
			settings = await api<Settings>(
				`/guilds/${guildId}/settings`,
				send('PATCH', { transcript_retention_days: retentionValue ? Number(retentionValue) : null })
			);
			toast('Settings saved');
		} catch (err) {
			toast(errorMessage(err), 'error');
		} finally {
			saving = false;
		}
	}
</script>

<PageHeader title="Settings" description="Server-wide options for how tickets are handled." />

<form onsubmit={save} class="mt-6 space-y-5">
	<section class="card space-y-5 p-5">
		<div>
			<h2 class="font-medium">Transcripts</h2>
			<p class="hint mt-1">
				Messages in tickets are saved so you can review them later. Choose how long to keep them
				after a ticket closes.
			</p>
		</div>
		<Field
			label="Keep transcripts for"
			for="retention"
			hint="Older transcripts are deleted automatically. Ticket details and stats are kept."
		>
			<select id="retention" class="input sm:w-56" bind:value={retentionValue} disabled={!settings}>
				{#each retention as o (o.value)}
					<option value={o.value}>{o.label}</option>
				{/each}
			</select>
		</Field>
	</section>

	<div class="flex justify-end">
		<button type="submit" class="btn btn-primary" disabled={!dirty || saving}>
			{saving ? 'Saving…' : 'Save settings'}
		</button>
	</div>
</form>
