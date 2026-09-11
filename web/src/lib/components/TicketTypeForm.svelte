<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import {
		api,
		ApiError,
		errorMessage,
		send,
		type Channel,
		type Role,
		type TicketMode,
		type TicketType,
		type TicketTypeInput
	} from '$lib/api';
	import { toast } from '$lib/toast.svelte';
	import ChannelSelect from './ChannelSelect.svelte';
	import Field from './Field.svelte';
	import Icon, { type IconName } from './Icon.svelte';
	import RolePicker from './RolePicker.svelte';

	let { guildId, initial }: { guildId: string; initial?: TicketType } = $props();

	const DEFAULT_WELCOME =
		'Thanks for reaching out, {user}! Tell us what you need help with and someone from the team will be with you shortly.';

	let form = $state<TicketTypeInput>(
		untrack(() => ({
			name: initial?.name ?? '',
			emoji: initial?.emoji ?? '',
			description: initial?.description ?? '',
			mode: initial?.mode ?? 'channel',
			parent_id: initial?.parent_id ?? null,
			support_role_ids: initial?.support_role_ids ?? [],
			name_format: initial?.name_format ?? 'ticket-{number}',
			welcome_message: initial?.welcome_message ?? '',
			max_open_per_user: initial?.max_open_per_user ?? 1
		}))
	);

	let channels = $state<Channel[]>([]);
	let roles = $state<Role[]>([]);
	let saving = $state(false);
	let errors = $state<Record<string, string>>({});

	onMount(async () => {
		try {
			[channels, roles] = await Promise.all([
				api<Channel[]>(`/guilds/${guildId}/channels`),
				api<Role[]>(`/guilds/${guildId}/roles`)
			]);
		} catch (e) {
			toast(errorMessage(e), 'error');
		}
	});

	const modes: { value: TicketMode; label: string; body: string; icon: IconName }[] = [
		{
			value: 'channel',
			label: 'Private channel',
			body: 'Each ticket gets its own channel. Classic and familiar.',
			icon: 'hash'
		},
		{
			value: 'thread',
			label: 'Private thread',
			body: 'Tickets open as threads in one channel. Tidier, with no channel limit.',
			icon: 'thread'
		}
	];

	function setMode(mode: TicketMode) {
		if (form.mode === mode) return;
		form.mode = mode;
		// A category doesn't make sense for threads and vice versa.
		form.parent_id = null;
	}

	const formatHint = $derived(
		'Use {number} and {username}. Preview: ' +
			(form.name_format || 'ticket-{number}')
				.replaceAll('{number}', '0042')
				.replaceAll('{username}', 'adam')
				.replaceAll('{user}', 'adam')
	);

	async function save(e: SubmitEvent) {
		e.preventDefault();
		saving = true;
		errors = {};
		try {
			if (initial) {
				const res = await api<{ warning: string }>(
					`/guilds/${guildId}/ticket-types/${initial.id}`,
					send('PATCH', form)
				);
				toast('Ticket type saved');
				if (res.warning) toast(res.warning, 'info');
			} else {
				await api<TicketType>(`/guilds/${guildId}/ticket-types`, send('POST', form));
				toast('Ticket type created');
			}
			goto(`/servers/${guildId}/ticket-types`);
		} catch (err) {
			if (err instanceof ApiError && err.field) errors = { [err.field]: err.message };
			else toast(errorMessage(err), 'error');
		} finally {
			saving = false;
		}
	}
</script>

<form onsubmit={save} class="space-y-5">
	<section class="card space-y-5 p-5">
		<h2 class="font-medium">Basics</h2>
		<div class="grid gap-5 sm:grid-cols-[1fr_7rem]">
			<Field label="Name" for="name" error={errors.name}>
				<input
					id="name"
					class="input"
					bind:value={form.name}
					maxlength="80"
					placeholder="e.g. General support"
					aria-invalid={!!errors.name}
					required
				/>
			</Field>
			<Field label="Emoji" for="emoji" optional error={errors.emoji}>
				<input
					id="emoji"
					class="input text-center"
					bind:value={form.emoji}
					placeholder="🎫"
					aria-invalid={!!errors.emoji}
				/>
			</Field>
		</div>
		<Field
			label="Description"
			for="description"
			optional
			hint="Shown under the option when a panel uses a dropdown."
			error={errors.description}
		>
			<input
				id="description"
				class="input"
				bind:value={form.description}
				maxlength="100"
				placeholder="e.g. Questions about the server or your account"
				aria-invalid={!!errors.description}
			/>
		</Field>
	</section>

	<section class="card space-y-5 p-5">
		<div>
			<h2 class="font-medium">Where tickets open</h2>
			<p class="hint mt-1">Only the member who opened the ticket and your support team can see it.</p>
		</div>
		<div class="grid gap-3 sm:grid-cols-2" role="radiogroup" aria-label="Ticket format">
			{#each modes as m (m.value)}
				{@const selected = form.mode === m.value}
				<button
					type="button"
					role="radio"
					aria-checked={selected}
					onclick={() => setMode(m.value)}
					class="flex gap-3 rounded-xl border p-4 text-left transition-colors {selected
						? 'border-accent bg-accent/5'
						: 'border-border hover:border-border-strong'}"
				>
					<span
						class="grid size-8 shrink-0 place-items-center rounded-lg bg-elevated {selected
							? 'text-accent'
							: 'text-muted'}"
					>
						<Icon name={m.icon} />
					</span>
					<span>
						<span class="block text-sm font-medium">{m.label}</span>
						<span class="mt-0.5 block text-xs text-muted">{m.body}</span>
					</span>
				</button>
			{/each}
		</div>

		{#if form.mode === 'channel'}
			<Field
				label="Category"
				for="parent"
				optional
				hint="New ticket channels are created in this category."
				error={errors.parent_id}
			>
				<ChannelSelect
					id="parent"
					{channels}
					kinds={['category']}
					bind:value={form.parent_id}
					placeholder="No category"
					invalid={!!errors.parent_id}
				/>
			</Field>
		{:else}
			<Field
				label="Channel"
				for="parent"
				hint="Threads are created in this channel. Members need to be able to see it, but each thread is private."
				error={errors.parent_id}
			>
				<ChannelSelect
					id="parent"
					{channels}
					kinds={['text']}
					bind:value={form.parent_id}
					placeholder="Select a channel"
					invalid={!!errors.parent_id}
				/>
			</Field>
		{/if}

		<Field
			label={form.mode === 'channel' ? 'Channel name' : 'Thread name'}
			for="name_format"
			hint={formatHint}
			error={errors.name_format}
		>
			<input
				id="name_format"
				class="input font-mono text-[13px]"
				bind:value={form.name_format}
				maxlength="90"
				aria-invalid={!!errors.name_format}
			/>
		</Field>
	</section>

	<section class="card space-y-5 p-5">
		<div>
			<h2 class="font-medium">Support team</h2>
			<p class="hint mt-1">
				These roles can see, claim and close tickets{form.mode === 'thread'
					? ", and are pinged into each new thread"
					: ''}. People with Manage Server always can.
			</p>
		</div>
		<Field label="Support roles" for="roles" error={errors.support_role_ids}>
			<RolePicker id="roles" {roles} bind:value={form.support_role_ids} />
		</Field>
	</section>

	<section class="card space-y-5 p-5">
		<h2 class="font-medium">Messages & limits</h2>
		<Field
			label="Welcome message"
			for="welcome"
			optional
			hint={'Posted when the ticket opens. Use {user} to mention the member.'}
			error={errors.welcome_message}
		>
			<textarea
				id="welcome"
				class="input"
				rows="4"
				bind:value={form.welcome_message}
				maxlength="2000"
				placeholder={DEFAULT_WELCOME}
				aria-invalid={!!errors.welcome_message}
			></textarea>
		</Field>
		<Field
			label="Open tickets per member"
			for="max_open"
			hint="How many tickets of this type one member can have open at once."
			error={errors.max_open_per_user}
		>
			<input
				id="max_open"
				type="number"
				min="1"
				max="10"
				class="input w-24"
				bind:value={form.max_open_per_user}
				aria-invalid={!!errors.max_open_per_user}
			/>
		</Field>
	</section>

	<div
		class="sticky bottom-4 flex items-center justify-end gap-2 rounded-xl border border-border bg-surface/90 p-3 shadow-2xl shadow-black/40 backdrop-blur"
	>
		<a href="/servers/{guildId}/ticket-types" class="btn btn-ghost">Cancel</a>
		<button type="submit" class="btn btn-primary" disabled={saving}>
			{saving ? 'Saving…' : initial ? 'Save changes' : 'Create ticket type'}
		</button>
	</div>
</form>
