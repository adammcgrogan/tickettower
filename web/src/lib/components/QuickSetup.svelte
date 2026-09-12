<script lang="ts">
	import {
		api,
		ApiError,
		errorMessage,
		send,
		type Channel,
		type Panel,
		type Role,
		type TicketMode,
		type TicketType
	} from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { emojiText } from '$lib/format';
	import { templates, typesFor } from '$lib/templates';
	import { toast } from '$lib/toast.svelte';
	import ChannelSelect from './ChannelSelect.svelte';
	import Field from './Field.svelte';
	import Icon from './Icon.svelte';
	import PanelPreview from './PanelPreview.svelte';
	import RolePicker from './RolePicker.svelte';
	import Segmented from './Segmented.svelte';

	/**
	 * Sets a server up in one step: ticket types from a template (or one named
	 * type), and with `publish`, ticket buttons for them posted in a channel.
	 * Home shows it for a new server; an empty ticket types page shows it
	 * without publishing.
	 */
	let {
		guildId,
		channels,
		roles,
		publish = true,
		ondone
	}: { guildId: string; channels: Channel[]; roles: Role[]; publish?: boolean; ondone: () => void } = $props();

	let templateId = $state('simple');
	let qs = $state({
		name: 'General support',
		mode: 'channel' as TicketMode,
		parent: null as string | null,
		roles: [] as string[],
		panelChannel: null as string | null
	});
	let errors = $state<Record<string, string>>({});
	let busy = $state(false);
	// Kept between attempts so a retry after a failure doesn't create
	// duplicates. Once anything exists, the template can't change.
	let created = $state<{ types: TicketType[]; panel?: Panel }>({ types: [] });
	const locked = $derived(created.types.length > 0);

	const template = $derived(templates.find((t) => t.id === templateId) ?? templates[0]);
	const types = $derived(typesFor(template, qs.name));
	const simple = $derived(template.types.length === 0);

	const modes: { value: TicketMode; label: string }[] = [
		{ value: 'channel', label: 'Private channels' },
		{ value: 'thread', label: 'Private threads' }
	];
	const parentKind = $derived<Channel['kind']>(qs.mode === 'thread' ? 'text' : 'category');
	const previewTypes = $derived(
		types.map((t, i) => ({ id: i, name: t.name || 'General support', emoji: t.emoji }) as TicketType)
	);
	const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		// A category chosen for channels doesn't carry over to threads.
		const parent = channels.some((c) => c.id === qs.parent && c.kind === parentKind) ? qs.parent : null;
		if (qs.mode === 'thread' && !parent) {
			errors = { parent_id: 'Choose the channel that ticket threads will be created in.' };
			return;
		}
		if (publish && !qs.panelChannel) {
			errors = { panel_channel: 'Choose where to post the buttons.' };
			return;
		}

		busy = true;
		const g = `/guilds/${guildId}`;
		try {
			for (let i = created.types.length; i < types.length; i++) {
				const t = await api<TicketType>(
					`${g}/ticket-types`,
					send('POST', { ...types[i], mode: qs.mode, parent_id: parent, support_role_ids: qs.roles })
				);
				created.types.push(t);
			}
			if (!publish) {
				toast(`Created ${plural(types.length, 'ticket type')}. Add them to ticket buttons so members can use them.`);
				ondone();
				return;
			}
			created.panel ??= await api<Panel>(
				`${g}/panels`,
				send('POST', { ...template.panel, color: 0xf2b544, ticket_type_ids: created.types.map((t) => t.id) })
			);
			await api<Panel>(`${g}/panels/${created.panel.id}/publish`, send('POST', { channel_id: qs.panelChannel }));
			const name = channels.find((c) => c.id === qs.panelChannel)?.name ?? 'your channel';
			toast(`Published. Members can open tickets from #${name}.`);
			ondone();
		} catch (err) {
			if (created.panel) errors = { panel_channel: errorMessage(err) };
			else if (err instanceof ApiError && err.field) errors = { [err.field]: err.message };
			else toast(errorMessage(err), 'error');
		} finally {
			busy = false;
		}
	}
</script>

<section class="overflow-hidden rounded-xl border border-border bg-surface">
	<div class="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_24rem]">
		<form onsubmit={submit} class="space-y-6 p-6 sm:p-8">
			<div>
				<h2 class="text-lg font-semibold">{publish ? 'Get your ticket buttons live' : 'Start with a template'}</h2>
				<p class="mt-1 max-w-lg text-sm text-muted">
					{#if publish}
						Answer a few questions and {APP_NAME} will create your ticket types and post buttons members can
						click to open a ticket. You can change everything afterwards.
					{:else}
						Create a few ticket types in one go, with questions and welcome messages ready to use. You can
						change everything afterwards.
					{/if}
				</p>
			</div>

			<fieldset>
				<legend class="label">What kind of server is it?</legend>
				<div class="mt-2 grid gap-2 sm:grid-cols-2">
					{#each templates as t (t.id)}
						{@const on = t.id === templateId}
						<label
							class="flex gap-3 rounded-lg border p-3 transition-colors has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-accent {on
								? 'border-border-strong bg-elevated'
								: 'border-border'} {locked && !on
								? 'cursor-not-allowed opacity-50'
								: 'cursor-pointer hover:border-border-strong'}"
						>
							<input
								type="radio"
								name="qs-template"
								value={t.id}
								bind:group={templateId}
								disabled={locked && !on}
								class="sr-only"
							/>
							<span class="min-w-0 flex-1">
								<span class="block text-sm font-medium">{t.name}</span>
								<span class="mt-0.5 block text-xs text-muted">{t.summary}</span>
							</span>
							{#if on}<Icon name="check" size={15} class="mt-0.5" />{/if}
						</label>
					{/each}
				</div>
			</fieldset>

			{#if simple}
				<Field
					label="What do members need help with?"
					for="qs-name"
					hint="This becomes the button members click."
					error={errors.name}
				>
					<input
						id="qs-name"
						class="input sm:max-w-sm"
						bind:value={qs.name}
						maxlength="80"
						required
						disabled={locked}
						aria-invalid={!!errors.name}
					/>
				</Field>
			{:else}
				<div>
					<p class="label">Ticket types it creates</p>
					<ul class="mt-2 divide-y divide-border rounded-lg border border-border">
						{#each types as t (t.name)}
							<li class="flex items-center gap-3 px-3 py-2.5">
								<span class="w-5 text-center" aria-hidden="true">{emojiText(t.emoji)}</span>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium">{t.name}</span>
									<span class="block truncate text-xs text-muted">{t.description}</span>
								</span>
								<span class="shrink-0 text-xs text-subtle">
									{t.questions.length ? plural(t.questions.length, 'question') : 'No questions'}
								</span>
							</li>
						{/each}
					</ul>
				</div>
			{/if}

			<Field
				label="Where should tickets open?"
				for="qs-parent"
				hint={qs.mode === 'channel'
					? 'Each ticket gets its own channel, in this category if you pick one.'
					: 'Tickets open as private threads inside this channel.'}
				help="channels-vs-threads"
				error={errors.parent_id}
			>
				<div class="flex flex-col gap-2 sm:flex-row sm:items-center">
					<Segmented options={modes} bind:value={qs.mode} label="Ticket format" />
					<div class="sm:w-60">
						<ChannelSelect
							id="qs-parent"
							{channels}
							kinds={[parentKind]}
							bind:value={qs.parent}
							placeholder={qs.mode === 'channel' ? 'No category' : 'Choose a channel'}
							invalid={!!errors.parent_id}
						/>
					</div>
				</div>
			</Field>

			<Field
				label="Who handles tickets?"
				for="qs-roles"
				optional
				hint="These roles can see and reply to every ticket. People with Manage Server always can."
				help="support-team"
				error={errors.support_role_ids}
			>
				<div class="sm:max-w-sm">
					<RolePicker id="qs-roles" {roles} bind:value={qs.roles} />
				</div>
			</Field>

			{#if publish}
				<Field
					label="Where should the buttons go?"
					for="qs-channel"
					hint="Usually a public channel, like #support."
					error={errors.panel_channel}
				>
					<div class="sm:max-w-sm">
						<ChannelSelect
							id="qs-channel"
							{channels}
							kinds={['text', 'announcement']}
							bind:value={qs.panelChannel}
							placeholder="Choose a channel"
							invalid={!!errors.panel_channel}
						/>
					</div>
				</Field>
			{/if}

			<div class="flex flex-wrap items-center gap-4 pt-1">
				<button type="submit" class="btn btn-primary" disabled={busy}>
					{#if publish}
						{busy ? 'Publishing…' : 'Create and publish'}
					{:else}
						{busy ? 'Creating…' : `Create ${plural(types.length, 'ticket type')}`}
					{/if}
				</button>
				<a href="/servers/{guildId}/ticket-types/new" class="text-sm text-muted hover:text-fg">
					{publish ? 'Set things up step by step instead' : 'Create one from scratch instead'}
				</a>
			</div>
		</form>

		<aside class="border-t border-border bg-bg/50 p-6 sm:p-8 lg:border-t-0 lg:border-l">
			<p class="mb-3 text-sm text-muted">What members will see</p>
			<PanelPreview
				title={template.panel.title}
				description={template.panel.description}
				color={0xf2b544}
				style={template.panel.style}
				types={previewTypes}
			/>
		</aside>
	</div>
</section>
