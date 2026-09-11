<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import {
		api,
		ApiError,
		errorMessage,
		hexToInt,
		intToHex,
		send,
		type Channel,
		type Panel,
		type PanelInput,
		type PanelStyle,
		type TicketType
	} from '$lib/api';
	import { discordURL, emojiText } from '$lib/format';
	import { toast } from '$lib/toast.svelte';
	import ChannelSelect from './ChannelSelect.svelte';
	import Dialog from './Dialog.svelte';
	import Field from './Field.svelte';
	import Icon from './Icon.svelte';
	import PanelPreview from './PanelPreview.svelte';
	import Segmented from './Segmented.svelte';

	let {
		guildId,
		initial,
		types,
		channels
	}: { guildId: string; initial?: Panel; types: TicketType[]; channels: Channel[] } = $props();

	const MAX_TYPES = 25;
	const swatches = [0x7c6cff, 0x5865f2, 0x3ecf8e, 0xf5c542, 0xf97316, 0xf0616d, 0xec4899, 0x5d5d66];
	const styles: { value: PanelStyle; label: string }[] = [
		{ value: 'buttons', label: 'Buttons' },
		{ value: 'dropdown', label: 'Dropdown' }
	];

	const toInput = (p: Panel): PanelInput => ({
		title: p.title,
		description: p.description,
		color: p.color,
		style: p.style,
		ticket_type_ids: [...p.ticket_type_ids]
	});

	let panel = $state<Panel | undefined>(untrack(() => initial));
	let form = $state<PanelInput>(
		untrack(() =>
			initial
				? toInput(initial)
				: {
						title: 'Need a hand?',
						description:
							"Pick a topic below and we'll open a private ticket for you. Our team will be with you shortly.",
						color: 0x7c6cff,
						style: 'buttons',
						ticket_type_ids: types.slice(0, MAX_TYPES).map((t) => t.id)
					}
		)
	);
	let saved = $state(untrack(() => JSON.stringify(form)));
	const dirty = $derived(JSON.stringify(form) !== saved);

	let saving = $state(false);
	let errors = $state<Record<string, string>>({});

	const selected = $derived(
		form.ticket_type_ids
			.map((id) => types.find((t) => t.id === id))
			.filter((t): t is TicketType => !!t)
	);
	const unselected = $derived(types.filter((t) => !form.ticket_type_ids.includes(t.id)));

	const publishedChannel = $derived(channels.find((c) => c.id === panel?.channel_id));

	function move(index: number, by: number) {
		const ids = [...form.ticket_type_ids];
		[ids[index], ids[index + by]] = [ids[index + by], ids[index]];
		form.ticket_type_ids = ids;
	}

	/** Saves the panel, returning it, or null if saving failed. */
	async function save(): Promise<Panel | null> {
		saving = true;
		errors = {};
		try {
			if (panel) {
				const res = await api<{ panel: Panel; warning: string }>(
					`/guilds/${guildId}/panels/${panel.id}`,
					send('PATCH', form)
				);
				panel = res.panel;
				if (res.warning) toast(res.warning, 'info');
			} else {
				panel = await api<Panel>(`/guilds/${guildId}/panels`, send('POST', form));
			}
			form = toInput(panel);
			saved = JSON.stringify(form);
			return panel;
		} catch (e) {
			if (e instanceof ApiError && e.field) errors = { [e.field]: e.message };
			else toast(errorMessage(e), 'error');
			return null;
		} finally {
			saving = false;
		}
	}

	async function onsubmit(e: SubmitEvent) {
		e.preventDefault();
		const isNew = !panel;
		const p = await save();
		if (!p) return;
		if (isNew) {
			toast('Panel created. Publish it to a channel when you are ready.');
			goto(`/servers/${guildId}/panels/${p.id}`, { replaceState: true });
		} else {
			toast(p.message_id ? 'Panel saved and updated in Discord' : 'Panel saved');
		}
	}

	// --- Publishing ---

	let publishOpen = $state(false);
	let publishChannel = $state<string | null>(null);
	let publishing = $state(false);
	let publishError = $state('');

	function openPublish() {
		publishChannel = panel?.channel_id ?? null;
		publishError = '';
		publishOpen = true;
	}

	async function publish() {
		if (!publishChannel) {
			publishError = 'Choose a channel.';
			return;
		}
		publishing = true;
		publishError = '';
		try {
			const p = dirty || !panel ? await save() : panel;
			if (!p) return;
			panel = await api<Panel>(
				`/guilds/${guildId}/panels/${p.id}/publish`,
				send('POST', { channel_id: publishChannel })
			);
			const name = channels.find((c) => c.id === publishChannel)?.name;
			toast(`Panel published in #${name ?? 'channel'}`);
			publishOpen = false;
		} catch (e) {
			publishError = errorMessage(e);
		} finally {
			publishing = false;
		}
	}
</script>

{#if panel}
	<div
		class="mb-5 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-surface px-4 py-3"
	>
		<div class="flex items-center gap-2.5 text-sm">
			{#if panel.message_id && panel.channel_id}
				<span class="size-2 rounded-full bg-success"></span>
				<span>Live in <span class="font-medium">#{publishedChannel?.name ?? 'unknown'}</span></span>
				<a
					href={discordURL(guildId, panel.channel_id, panel.message_id)}
					target="_blank"
					rel="noopener"
					class="inline-flex items-center gap-1 text-xs text-muted transition-colors hover:text-fg"
				>
					View in Discord <Icon name="external" size={12} />
				</a>
			{:else}
				<span class="size-2 rounded-full bg-subtle"></span>
				<span class="text-muted">Not published yet</span>
			{/if}
		</div>
		<button type="button" class="btn btn-primary h-8 px-3" onclick={openPublish}>
			<Icon name="send" size={14} />
			{panel.message_id ? 'Republish' : 'Publish'}
		</button>
	</div>
{/if}

<div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,26rem)]">
	<form {onsubmit} class="space-y-5">
		<section class="card space-y-5 p-5">
			<h2 class="font-medium">Message</h2>
			<Field label="Title" for="title" error={errors.title}>
				<input
					id="title"
					class="input"
					bind:value={form.title}
					maxlength="256"
					required
					aria-invalid={!!errors.title}
				/>
			</Field>
			<Field
				label="Description"
				for="description"
				optional
				hint="Supports Discord markdown like **bold** and [links](https://example.com)."
				error={errors.description}
			>
				<textarea
					id="description"
					class="input"
					rows="4"
					bind:value={form.description}
					maxlength="4000"
					aria-invalid={!!errors.description}
				></textarea>
			</Field>
			<Field label="Colour" for="color" error={errors.color}>
				<div class="flex flex-wrap items-center gap-2">
					<input
						id="color"
						type="color"
						value={intToHex(form.color)}
						oninput={(e) => (form.color = hexToInt(e.currentTarget.value))}
						class="h-9 w-11 cursor-pointer rounded-lg border border-border bg-bg p-1"
					/>
					{#each swatches as c (c)}
						<button
							type="button"
							onclick={() => (form.color = c)}
							class="size-7 rounded-full ring-offset-2 ring-offset-surface transition {form.color === c
								? 'ring-2 ring-fg'
								: 'hover:scale-110'}"
							style="background:{intToHex(c)}"
							aria-label="Use colour {intToHex(c)}"
						></button>
					{/each}
				</div>
			</Field>
		</section>

		<section class="card space-y-5 p-5">
			<div class="flex flex-wrap items-center justify-between gap-3">
				<div>
					<h2 class="font-medium">Ticket types</h2>
					<p class="hint mt-1">Members pick one of these to open a ticket.</p>
				</div>
				<Segmented options={styles} bind:value={form.style} label="Panel style" />
			</div>

			{#if types.length === 0}
				<p class="rounded-lg border border-dashed border-border p-4 text-sm text-muted">
					You haven't created any ticket types yet.
					<a href="/servers/{guildId}/ticket-types/new" class="text-accent hover:underline">
						Create one first
					</a>.
				</p>
			{:else}
				{#if selected.length > 0}
					<ul class="divide-y divide-border rounded-lg border border-border">
						{#each selected as t, i (t.id)}
							<li class="flex items-center gap-3 px-3 py-2">
								<span class="w-6 text-center">{t.emoji ? emojiText(t.emoji) : '·'}</span>
								<span class="min-w-0 flex-1 truncate text-sm">{t.name}</span>
								<button
									type="button"
									class="btn btn-ghost size-7 p-0"
									disabled={i === 0}
									onclick={() => move(i, -1)}
									aria-label="Move {t.name} up"><Icon name="chevron-up" size={14} /></button
								>
								<button
									type="button"
									class="btn btn-ghost size-7 p-0"
									disabled={i === selected.length - 1}
									onclick={() => move(i, 1)}
									aria-label="Move {t.name} down"><Icon name="chevron-down" size={14} /></button
								>
								<button
									type="button"
									class="btn btn-ghost size-7 p-0"
									onclick={() => (form.ticket_type_ids = form.ticket_type_ids.filter((id) => id !== t.id))}
									aria-label="Remove {t.name}"><Icon name="x" size={14} /></button
								>
							</li>
						{/each}
					</ul>
				{/if}
				{#if unselected.length > 0 && form.ticket_type_ids.length < MAX_TYPES}
					<div class="flex flex-wrap gap-2">
						{#each unselected as t (t.id)}
							<button
								type="button"
								onclick={() => (form.ticket_type_ids = [...form.ticket_type_ids, t.id])}
								class="inline-flex items-center gap-1.5 rounded-md border border-dashed border-border-strong px-2.5 py-1 text-xs text-muted transition-colors hover:border-accent hover:text-fg"
							>
								<Icon name="plus" size={12} />
								{t.name}
							</button>
						{/each}
					</div>
				{/if}
			{/if}
			{#if errors.ticket_type_ids}<p class="text-xs text-danger">{errors.ticket_type_ids}</p>{/if}
		</section>

		<div
			class="sticky bottom-4 flex items-center justify-between gap-2 rounded-xl border border-border bg-surface/90 p-3 shadow-2xl shadow-black/40 backdrop-blur"
		>
			<span class="pl-1 text-xs text-muted">{dirty ? 'Unsaved changes' : panel ? 'All changes saved' : ''}</span>
			<div class="flex gap-2">
				<a href="/servers/{guildId}/panels" class="btn btn-ghost">Back</a>
				<button type="submit" class="btn btn-primary" disabled={saving || (!!panel && !dirty)}>
					{saving ? 'Saving…' : panel ? 'Save changes' : 'Create panel'}
				</button>
			</div>
		</div>
	</form>

	<aside class="lg:sticky lg:top-20 lg:self-start">
		<div class="mb-2 text-xs font-medium tracking-wide text-subtle uppercase">Preview</div>
		<PanelPreview
			title={form.title}
			description={form.description}
			color={form.color}
			style={form.style}
			types={selected}
		/>
	</aside>
</div>

<Dialog
	bind:open={publishOpen}
	title={panel?.message_id ? 'Republish panel' : 'Publish panel'}
	description={panel?.message_id
		? 'Publishing to the same channel updates the existing message. Choosing a new channel moves it.'
		: 'The bot will post this panel so members can start opening tickets.'}
>
	<Field label="Channel" for="publish-channel" error={publishError}>
		<ChannelSelect
			id="publish-channel"
			{channels}
			kinds={['text', 'announcement']}
			bind:value={publishChannel}
			invalid={!!publishError}
		/>
	</Field>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (publishOpen = false)}>Cancel</button>
		<button class="btn btn-primary" onclick={publish} disabled={publishing}>
			{publishing ? 'Publishing…' : dirty ? 'Save & publish' : 'Publish'}
		</button>
	{/snippet}
</Dialog>
