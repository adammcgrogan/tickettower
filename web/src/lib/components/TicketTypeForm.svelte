<script lang="ts">
	import { getContext, onMount, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import {
		api,
		ApiError,
		errorMessage,
		CLOSED_KEEP_DAYS,
		MAX_BUTTON_LABEL,
		MAX_QUESTIONS,
		MAX_RATING_PROMPT,
		MINUTE_OPTIONS,
		minutesLabel,
		send,
		type ButtonStyle,
		type Channel,
		type ClaimLock,
		type Guild,
		type Panel,
		type PanelInput,
		type QuestionStyle,
		type Role,
		type TicketMode,
		type TicketType,
		type TicketTypeInput
	} from '$lib/api';
	import {
		answerPlaceholders,
		fillPlaceholders,
		namePlaceholders,
		ratingPlaceholders,
		slug,
		welcomePlaceholders
	} from '$lib/placeholders';
	import { toast } from '$lib/toast.svelte';
	import ChannelSelect from './ChannelSelect.svelte';
	import Field from './Field.svelte';
	import Icon, { type IconName } from './Icon.svelte';
	import PlaceholderChips from './PlaceholderChips.svelte';
	import RolePicker from './RolePicker.svelte';
	import Segmented from './Segmented.svelte';
	import WelcomePreview from './WelcomePreview.svelte';

	let {
		guildId,
		initial,
		copyOf,
		onsaved
	}: {
		guildId: string;
		initial?: TicketType;
		/** A type to fill a new form from, for duplicating it. Nothing is saved until Create. */
		copyOf?: TicketType;
		onsaved?: (name: string) => void;
	} = $props();

	const NAME_MAX = 80;
	// The settings the form starts from: the type being edited, or the one being copied.
	const source = untrack(() => initial ?? copyOf);
	const startName = untrack(() =>
		copyOf && !initial ? `${copyOf.name.slice(0, NAME_MAX - ' (copy)'.length)} (copy)` : (initial?.name ?? '')
	);

	const DEFAULT_WELCOME =
		'Thanks for reaching out, {user}! Tell us what you need help with and someone from the team will be with you shortly.';
	// Matches defaultRatingPrompt in the bot.
	const DEFAULT_RATING_PROMPT = 'How did we do? Rate your experience below.';

	let form = $state<TicketTypeInput>(
		untrack(() => ({
			name: startName,
			emoji: source?.emoji ?? '',
			description: source?.description ?? '',
			mode: source?.mode ?? 'channel',
			parent_id: source?.parent_id ?? null,
			support_role_ids: [...(source?.support_role_ids ?? [])],
			name_format: source?.name_format ?? 'ticket-{number}',
			welcome_message: source?.welcome_message ?? '',
			max_open_per_user: source?.max_open_per_user ?? 1,
			questions: source?.questions.map((q) => ({ ...q })) ?? [],
			auto_close_hours: source?.auto_close_hours ?? null,
			required_role_ids: [...(source?.required_role_ids ?? [])],
			blocked_role_ids: [...(source?.blocked_role_ids ?? [])],
			cooldown_minutes: source?.cooldown_minutes ?? 0,
			ask_rating: source?.ask_rating ?? true,
			rating_prompt: source?.rating_prompt ?? '',
			button_style: source?.button_style ?? 'primary',
			button_label: source?.button_label ?? '',
			claim_lock: source?.claim_lock ?? 'off',
			claim_lock_exempt_role_ids: [...(source?.claim_lock_exempt_role_ids ?? [])],
			reply_target_minutes: source?.reply_target_minutes ?? null,
			reminder_minutes: source?.reminder_minutes ?? null,
			reminder_repeat: source?.reminder_repeat ?? false,
			reminder_where: source?.reminder_where ?? 'ticket',
			reminder_ping: source?.reminder_ping ?? 'claimer',
			closed_parent_id: source?.closed_parent_id ?? null,
			closed_member_access: source?.closed_member_access ?? 'read',
			closed_keep_days: source?.closed_keep_days ?? 7
		}))
	);

	const closedAccess: { value: 'read' | 'hidden'; label: string; body: string }[] = [
		{ value: 'read', label: 'Can read it', body: "The member sees the closed ticket but can't write in it." },
		{ value: 'hidden', label: 'Hidden from them', body: 'Only the support team can see the closed ticket.' }
	];
	const keepDaysLabel = (d: number) => (d === 1 ? '1 day' : `${d} days`);

	const autoCloseOptions = [
		{ value: '', label: 'Never' },
		{ value: '12', label: 'After 12 hours' },
		{ value: '24', label: 'After 1 day' },
		{ value: '48', label: 'After 2 days' },
		{ value: '72', label: 'After 3 days' },
		{ value: '168', label: 'After 1 week' }
	];

	// Matches cooldownOptions in the API.
	const cooldownOptions = [
		{ value: 0, label: 'No wait' },
		{ value: 5, label: '5 minutes' },
		{ value: 15, label: '15 minutes' },
		{ value: 30, label: '30 minutes' },
		{ value: 60, label: '1 hour' },
		{ value: 360, label: '6 hours' },
		{ value: 1440, label: '1 day' }
	];

	const hoursLabel = (h: number) =>
		h % 24 === 0 ? `${h / 24} day${h === 24 ? '' : 's'}` : `${h} hours`;

	// Matches the bot: members are warned a quarter of the window ahead, at most a day.
	const autoCloseHint = $derived.by(() => {
		const h = form.auto_close_hours;
		if (!h) return 'Tickets stay open until someone closes them.';
		const lead = Math.min(h / 4, 24);
		return `Only when your team is waiting on the member. They're reminded ${hoursLabel(lead)} before, and any message keeps the ticket open.`;
	});

	// Discord's button colours, named as members see them.
	const buttonStyles: { value: ButtonStyle; label: string; swatch: string }[] = [
		{ value: 'primary', label: 'Blurple', swatch: '#5865f2' },
		{ value: 'secondary', label: 'Grey', swatch: '#4e5058' },
		{ value: 'success', label: 'Green', swatch: '#248046' },
		{ value: 'danger', label: 'Red', swatch: '#da373c' }
	];

	const claimLocks: { value: ClaimLock; label: string; body: string }[] = [
		{ value: 'off', label: 'Nothing changes', body: 'Everyone on the team can still read and reply.' },
		{ value: 'read_only', label: 'Others read only', body: 'The rest of the team can follow along but not reply.' },
		{ value: 'hidden', label: 'Others lose access', body: 'Only the claimer and the member can see the ticket.' }
	];
	const minuteChoices = [{ value: '', label: 'None' }, ...MINUTE_OPTIONS.map((m) => ({ value: String(m), label: minutesLabel(m) }))];
	const reminderChoices = [{ value: '', label: "Don't remind" }, ...MINUTE_OPTIONS.map((m) => ({ value: String(m), label: `After ${minutesLabel(m)}` }))];
	const reminderWheres = [
		{ value: 'ticket', label: 'In the ticket' },
		{ value: 'log', label: 'In the log channel' },
		{ value: 'both', label: 'Both' }
	];
	const reminderPings = [
		{ value: 'claimer', label: 'The claimer, or the support roles if unclaimed' },
		{ value: 'roles', label: 'The support roles' },
		{ value: 'none', label: 'Nobody' }
	];
	const minutesOrNull = (v: string) => (v ? Number(v) : null);

	// A claim lock needs channel tickets, since threads can't restrict a role.
	$effect(() => {
		if (form.mode === 'thread' && form.claim_lock !== 'off') form.claim_lock = 'off';
	});

	const answerStyles: { value: QuestionStyle; label: string }[] = [
		{ value: 'short', label: 'Short answer' },
		{ value: 'paragraph', label: 'Paragraph' }
	];

	function addQuestion() {
		form.questions.push({ label: '', placeholder: '', style: 'short', required: true });
	}

	function moveQuestion(i: number, by: -1 | 1) {
		const qs = form.questions;
		[qs[i], qs[i + by]] = [qs[i + by], qs[i]];
	}

	let channels = $state<Channel[]>([]);
	let roles = $state<Role[]>([]);
	// Only support roles can be exempt from a claim lock.
	const supportRolesOnly = $derived(roles.filter((r) => form.support_role_ids.includes(r.id)));
	let saving = $state(false);
	let errors = $state<Record<string, string>>({});

	// Which ticket buttons show this type. A new type can go straight onto
	// existing ones, so it isn't forgotten; with just one set, that's the
	// obvious choice.
	const MAX_PANEL_TYPES = 25;
	let panels = $state<Panel[] | null>(null);
	let addTo = $state<number[]>([]);
	const onPanel = (p: Panel) => !!initial && p.ticket_type_ids.includes(initial.id);
	const isFull = (p: Panel) => !onPanel(p) && p.ticket_type_ids.length >= MAX_PANEL_TYPES;
	// Ticket buttons need at least one type, so their only one can't be taken off.
	const isOnly = (p: Panel) => onPanel(p) && p.ticket_type_ids.length === 1;

	const snapshot = () => JSON.stringify([form, [...addTo].sort()]);
	let saved = $state('');
	const dirty = $derived(!!initial && saved !== '' && snapshot() !== saved);

	onMount(async () => {
		api<Panel[]>(`/guilds/${guildId}/panels`)
			.then((p) => {
				panels = p;
				if (initial) addTo = p.filter(onPanel).map((x) => x.id);
				// A copy starts on the same ticket buttons as the original, where there's room.
				else if (copyOf) addTo = p.filter((x) => x.ticket_type_ids.includes(copyOf.id) && !isFull(x)).map((x) => x.id);
				else if (p.length === 1 && !isFull(p[0])) addTo = [p[0].id];
				saved = snapshot();
			})
			.catch(() => (panels = []));
		try {
			[channels, roles] = await Promise.all([
				api<Channel[]>(`/guilds/${guildId}/channels`),
				api<Role[]>(`/guilds/${guildId}/roles`)
			]);
		} catch (e) {
			toast(errorMessage(e), 'error');
		}
	});

	/** Puts the type on, or takes it off, the ticket buttons that changed, returning how many failed. */
	async function syncPanels(typeId: number): Promise<number> {
		let failed = 0;
		for (const p of panels ?? []) {
			const want = addTo.includes(p.id);
			if (want === p.ticket_type_ids.includes(typeId)) continue;
			const body: PanelInput = {
				title: p.title,
				description: p.description,
				color: p.color,
				style: p.style,
				image_url: p.image_url,
				thumbnail_url: p.thumbnail_url,
				placeholder: p.placeholder,
				ticket_type_ids: want ? [...p.ticket_type_ids, typeId] : p.ticket_type_ids.filter((id) => id !== typeId)
			};
			try {
				const res = await api<{ panel: Panel; warning: string }>(
					`/guilds/${guildId}/panels/${p.id}`,
					send('PATCH', body)
				);
				panels = (panels ?? []).map((x) => (x.id === p.id ? res.panel : x));
				if (res.warning) toast(res.warning, 'info');
			} catch {
				failed++;
			}
		}
		return failed;
	}

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

	const getGuild = getContext<() => Guild | null>('guild');

	let nameInput = $state<HTMLInputElement>();
	let welcomeInput = $state<HTMLTextAreaElement>();
	let ratingInput = $state<HTMLTextAreaElement>();
	const answerTokens = $derived(answerPlaceholders(form.questions));

	// Matches the bot's channelName: text is slugged and stray hyphens trimmed.
	const namePreview = $derived(
		fillPlaceholders(form.name_format || 'ticket-{number}', {
			number: '0042',
			username: 'member',
			user: 'member',
			type: slug(form.name) || 'support',
			...Object.fromEntries(
				form.questions.map((q, i) => [`answer${i + 1}`, slug(q.placeholder) || `answer-${i + 1}`])
			)
		}).replace(/^[\s-]+|[\s-]+$/g, '') || 'ticket-0042'
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
				const failed = await syncPanels(initial.id);
				if (failed) toast("Saved, but your ticket buttons couldn't all be updated. Try again from Ticket buttons.", 'error');
				else toast('Ticket type saved');
				if (res.warning) toast(res.warning, 'info');
				saved = snapshot();
				onsaved?.(form.name);
			} else {
				const created = await api<TicketType>(`/guilds/${guildId}/ticket-types`, send('POST', form));
				const failed = await syncPanels(created.id);
				if (failed) {
					toast("Ticket type created, but it couldn't be added to your ticket buttons. Add it from Ticket buttons.", 'error');
				} else {
					toast(addTo.length ? 'Ticket type created and added to your ticket buttons' : 'Ticket type created');
				}
				goto(`/servers/${guildId}/ticket-types`);
			}
		} catch (err) {
			if (err instanceof ApiError && err.field) {
				errors = { [err.field]: err.message };
				tab = fieldTab[err.field] ?? tab;
			} else toast(errorMessage(err), 'error');
		} finally {
			saving = false;
		}
	}

	// --- Tabs ---

	type Tab = 'basics' | 'button' | 'welcome' | 'access' | 'handling' | 'closing';
	const tabs: { id: Tab; label: string }[] = [
		{ id: 'basics', label: 'Basics' },
		{ id: 'button', label: 'Button' },
		{ id: 'welcome', label: 'Questions and welcome' },
		{ id: 'access', label: 'Who can open' },
		{ id: 'handling', label: 'While open' },
		{ id: 'closing', label: 'Closing' }
	];
	let tab = $state<Tab>('basics');

	// Where each field lives, so a save error shows the tab it's on.
	const fieldTab: Record<string, Tab> = {
		name: 'basics',
		emoji: 'basics',
		description: 'basics',
		mode: 'basics',
		parent_id: 'basics',
		name_format: 'basics',
		support_role_ids: 'basics',
		button_label: 'button',
		button_style: 'button',
		questions: 'welcome',
		welcome_message: 'welcome',
		required_role_ids: 'access',
		blocked_role_ids: 'access',
		max_open_per_user: 'access',
		cooldown_minutes: 'access',
		claim_lock: 'handling',
		claim_lock_exempt_role_ids: 'handling',
		reply_target_minutes: 'handling',
		reminder_minutes: 'handling',
		reminder_where: 'handling',
		reminder_ping: 'handling',
		auto_close_hours: 'handling',
		ask_rating: 'closing',
		rating_prompt: 'closing',
		closed_parent_id: 'closing',
		closed_member_access: 'closing',
		closed_keep_days: 'closing'
	};
	const tabHasError = (t: Tab) => Object.keys(errors).some((f) => fieldTab[f] === t);

	let tabButtons = $state<Record<string, HTMLButtonElement>>({});
	function tabKey(e: KeyboardEvent, i: number) {
		const by = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
		if (!by) return;
		e.preventDefault();
		const next = tabs[(i + by + tabs.length) % tabs.length].id;
		tab = next;
		tabButtons[next]?.focus();
	}

	const buttonSwatch = $derived(buttonStyles.find((b) => b.value === form.button_style)?.swatch);
</script>

{#snippet heading(title: string, hint?: string)}
	<div>
		<h2 class="font-medium">{title}</h2>
		{#if hint}<p class="hint mt-1">{hint}</p>{/if}
	</div>
{/snippet}

<div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,24rem)]">
	<form onsubmit={save} class="min-w-0">
		<div role="tablist" aria-label="Ticket type settings" class="flex gap-1 overflow-x-auto border-b border-border">
			{#each tabs as t, i (t.id)}
				{@const active = tab === t.id}
				<button
					type="button"
					role="tab"
					id="tab-{t.id}"
					aria-selected={active}
					aria-controls="tabpanel"
					tabindex={active ? 0 : -1}
					bind:this={tabButtons[t.id]}
					onclick={() => (tab = t.id)}
					onkeydown={(e) => tabKey(e, i)}
					class="-mb-px flex shrink-0 items-center gap-1.5 border-b-2 px-3 py-2.5 text-sm whitespace-nowrap transition-colors {active
						? 'border-fg font-medium text-fg'
						: 'border-transparent text-muted hover:text-fg'}"
				>
					{t.label}
					{#if tabHasError(t.id)}<span class="size-1.5 rounded-full bg-danger" aria-label="Has a problem"></span>{/if}
				</button>
			{/each}
		</div>

		<div id="tabpanel" role="tabpanel" aria-labelledby="tab-{tab}" class="card mt-5 divide-y divide-border">
			{#if tab === 'basics'}
				<section class="space-y-5 p-5">
					<div class="grid gap-5 sm:grid-cols-[1fr_7rem]">
						<Field label="Name" for="name" error={errors.name}>
							<input
								id="name"
								class="input"
								bind:value={form.name}
								maxlength={NAME_MAX}
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
						hint="Shown under the option when ticket buttons are shown as a dropdown menu."
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

				<section class="space-y-5 p-5">
					{@render heading(
						'Where tickets open',
						'Only the member who opened the ticket and your support team can see it.'
					)}
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
							help="channels-vs-threads"
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
							help="channels-vs-threads"
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
						hint="Preview: {namePreview}"
						help="ticket-types"
						error={errors.name_format}
					>
						<input
							id="name_format"
							class="input font-mono text-[13px]"
							bind:this={nameInput}
							bind:value={form.name_format}
							maxlength="90"
							aria-invalid={!!errors.name_format}
						/>
						<PlaceholderChips
							items={[...namePlaceholders, ...answerTokens]}
							target={nameInput}
							bind:value={form.name_format}
						/>
					</Field>
				</section>

				<section class="space-y-5 p-5">
					{@render heading(
						'Support team',
						`These roles can see, claim and close tickets${form.mode === 'thread' ? ', and are pinged into each new thread' : ''}. People with Manage Server always can.`
					)}
					<Field
						label="Support roles"
						for="roles"
						hint={form.support_role_ids.length
							? undefined
							: 'With no support roles, only people with Manage Server can see these tickets.'}
						help="support-team"
						error={errors.support_role_ids}
					>
						<RolePicker id="roles" {roles} bind:value={form.support_role_ids} />
					</Field>
				</section>
			{:else if tab === 'button'}
				<section class="space-y-5 p-5">
					{@render heading('The button', 'How this type looks on your ticket buttons message.')}
					<div class="grid gap-5 sm:grid-cols-[1fr_auto]">
						<Field
							label="Button label"
							for="button_label"
							optional
							hint="Leave empty to use the name."
							error={errors.button_label}
						>
							<input
								id="button_label"
								class="input"
								bind:value={form.button_label}
								maxlength={MAX_BUTTON_LABEL}
								placeholder={form.name || 'e.g. Get help'}
								aria-invalid={!!errors.button_label}
							/>
						</Field>
						<Field label="Button colour" for="button_style" error={errors.button_style}>
							<div class="flex gap-2" role="radiogroup" id="button_style" aria-label="Button colour">
								{#each buttonStyles as b (b.value)}
									{@const on = form.button_style === b.value}
									<button
										type="button"
										role="radio"
										aria-checked={on}
										title={b.label}
										aria-label={b.label}
										onclick={() => (form.button_style = b.value)}
										class="grid h-9 w-11 place-items-center rounded-lg border transition-colors {on
											? 'border-fg'
											: 'border-border hover:border-border-strong'}"
									>
										<span class="h-4 w-6 rounded-sm" style="background:{b.swatch}"></span>
									</button>
								{/each}
							</div>
						</Field>
					</div>
				</section>

				<section class="space-y-4 p-5">
					{@render heading(
						'Shown on',
						'Members can only open this type from ticket buttons it’s on. Published buttons update in Discord straight away.'
					)}
					{#if panels === null}
						<div class="h-16 animate-pulse rounded-lg bg-elevated"></div>
					{:else if panels.length === 0}
						<p class="rounded-lg border border-dashed border-border p-4 text-sm text-muted">
							You haven't made any ticket buttons yet.
							<a href="/servers/{guildId}/buttons/new" class="text-fg underline-offset-4 hover:underline">
								Create them
							</a>
							once this type is saved.
						</p>
					{:else}
						<ul class="divide-y divide-border rounded-lg border border-border">
							{#each panels as p (p.id)}
								{@const locked = isFull(p) || isOnly(p)}
								<li>
									<label class="flex items-center gap-3 px-3 py-2.5 text-sm {locked ? '' : 'cursor-pointer'}">
										<input
											type="checkbox"
											class="size-4 accent-accent"
											disabled={locked}
											checked={addTo.includes(p.id)}
											onchange={(e) =>
												(addTo = e.currentTarget.checked
													? [...addTo, p.id]
													: addTo.filter((id) => id !== p.id))}
										/>
										<span class="min-w-0 flex-1 truncate {locked && !addTo.includes(p.id) ? 'text-muted' : ''}">
											{p.title}
										</span>
										<span class="shrink-0 text-xs text-subtle">
											{#if isFull(p)}Full{:else if isOnly(p)}Its only type{:else if p.message_id}Published{:else}Draft{/if}
										</span>
									</label>
								</li>
							{/each}
						</ul>
						{#if addTo.length === 0}
							<p class="flex items-center gap-1.5 text-xs text-muted">
								<Icon name="alert" size={13} /> Not on any ticket buttons, so members can't open it yet.
							</p>
						{/if}
					{/if}
				</section>
			{:else if tab === 'welcome'}
				<section class="space-y-5 p-5">
					<div class="flex items-start justify-between gap-4">
						<div>
							<h2 class="font-medium">Questions</h2>
							<p class="hint mt-1">
								Asked in a pop-up before the ticket opens. The answers are posted in the welcome message,
								so your team has the details up front.
								<a href="/help/forms" target="_blank" class="whitespace-nowrap text-fg underline-offset-4 hover:underline">
									Learn more
								</a>
							</p>
						</div>
						{#if form.questions.length > 0 && form.questions.length < MAX_QUESTIONS}
							<button type="button" class="btn btn-secondary h-8 shrink-0 px-3" onclick={addQuestion}>
								<Icon name="plus" size={14} /> Add question
							</button>
						{/if}
					</div>

					{#if form.questions.length === 0}
						<div class="rounded-xl border border-dashed border-border px-6 py-8 text-center">
							<p class="text-sm font-medium">No questions</p>
							<p class="mx-auto mt-1 max-w-sm text-sm text-muted">
								Members go straight to their ticket. Add up to {MAX_QUESTIONS} questions, like an order number
								or what they need help with.
							</p>
							<button type="button" class="btn btn-secondary mt-4 h-8 px-3" onclick={addQuestion}>
								<Icon name="plus" size={14} /> Add question
							</button>
						</div>
					{:else}
						<ol class="space-y-3">
							{#each form.questions as q, i (i)}
								<li class="space-y-4 rounded-lg border border-border bg-bg/40 p-4">
									<div class="flex items-center gap-2">
										<span class="text-xs font-medium text-muted">Question {i + 1}</span>
										<div class="ml-auto flex items-center gap-0.5">
											<button
												type="button"
												class="btn btn-ghost h-7 px-1.5"
												disabled={i === 0}
												onclick={() => moveQuestion(i, -1)}
												aria-label="Move question {i + 1} up"
											>
												<Icon name="chevron-up" size={14} />
											</button>
											<button
												type="button"
												class="btn btn-ghost h-7 px-1.5"
												disabled={i === form.questions.length - 1}
												onclick={() => moveQuestion(i, 1)}
												aria-label="Move question {i + 1} down"
											>
												<Icon name="chevron-down" size={14} />
											</button>
											<button
												type="button"
												class="btn btn-ghost h-7 px-1.5 hover:text-danger"
												onclick={() => form.questions.splice(i, 1)}
												aria-label="Remove question {i + 1}"
											>
												<Icon name="trash" size={14} />
											</button>
										</div>
									</div>
									<div class="grid gap-4 sm:grid-cols-2">
										<Field label="Question" for="q-{i}-label">
											<input
												id="q-{i}-label"
												class="input"
												bind:value={q.label}
												maxlength="45"
												placeholder="e.g. What's your order number?"
												required
											/>
										</Field>
										<Field label="Placeholder" for="q-{i}-placeholder" optional>
											<input
												id="q-{i}-placeholder"
												class="input"
												bind:value={q.placeholder}
												maxlength="100"
												placeholder="e.g. #12345"
											/>
										</Field>
									</div>
									<div class="flex flex-wrap items-center justify-between gap-3">
										<Segmented label="Answer length" options={answerStyles} bind:value={q.style} />
										<label class="flex cursor-pointer items-center gap-2 text-sm">
											<input type="checkbox" class="size-4 accent-accent" bind:checked={q.required} />
											Required
										</label>
									</div>
								</li>
							{/each}
						</ol>
					{/if}
					{#if errors.questions}<p class="text-xs text-danger">{errors.questions}</p>{/if}
				</section>

				<section class="space-y-5 p-5">
					<Field
						label="Welcome message"
						for="welcome"
						optional
						hint="Posted when the ticket opens. Click a placeholder to add it; mentions here don't ping anyone."
						help="ticket-types"
						error={errors.welcome_message}
					>
						<textarea
							id="welcome"
							class="input"
							rows="4"
							bind:this={welcomeInput}
							bind:value={form.welcome_message}
							maxlength="2000"
							placeholder={DEFAULT_WELCOME}
							aria-invalid={!!errors.welcome_message}
						></textarea>
						<PlaceholderChips
							items={[...welcomePlaceholders, ...answerTokens]}
							target={welcomeInput}
							bind:value={form.welcome_message}
						/>
					</Field>
				</section>
			{:else if tab === 'access'}
				<section class="space-y-5 p-5">
					{@render heading(
						'Roles',
						"Anyone who can see your ticket buttons can open this type, unless you narrow it down here. Members who don't qualify are told why when they click."
					)}
					<Field
						label="Required roles"
						for="required_roles"
						optional
						hint={form.required_role_ids.length
							? 'Members need at least one of these roles.'
							: 'Leave empty to let any member open this type.'}
						help="ticket-types"
						error={errors.required_role_ids}
					>
						<RolePicker id="required_roles" {roles} bind:value={form.required_role_ids} />
					</Field>
					<Field
						label="Blocked roles"
						for="blocked_roles"
						optional
						hint="Members with any of these roles can't open this type, such as a Muted role."
						error={errors.blocked_role_ids}
					>
						<RolePicker id="blocked_roles" {roles} bind:value={form.blocked_role_ids} />
					</Field>
					<p class="hint">
						To stop one person opening any ticket, use <code>/ticket block</code> in Discord or the blocked
						list in <a href="/servers/{guildId}/settings" class="text-fg underline-offset-4 hover:underline">Settings</a>.
					</p>
				</section>

				<section class="space-y-5 p-5">
					{@render heading('Limits')}
					<div class="grid gap-5 sm:grid-cols-2">
						<Field
							label="Open tickets per member"
							for="max_open"
							hint="How many of these one member can have open at once."
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
						<Field
							label="Wait between tickets"
							for="cooldown"
							hint="After one closes, before the same member can open another. Stops open, close, open again."
							error={errors.cooldown_minutes}
						>
							<select id="cooldown" class="input" bind:value={form.cooldown_minutes} aria-invalid={!!errors.cooldown_minutes}>
								{#each cooldownOptions as o (o.value)}
									<option value={o.value}>{o.label}</option>
								{/each}
							</select>
						</Field>
					</div>
				</section>
			{:else if tab === 'handling'}
				<section class="space-y-5 p-5">
					<div>
						<h2 class="font-medium">Claiming</h2>
						<p class="hint mt-1">
							What happens to the rest of the support team once someone claims a ticket.
							{#if form.mode === 'thread'}
								Private threads can't limit a role's access, so this only applies to channel tickets.
							{/if}
						</p>
					</div>
					<div class="grid gap-3 sm:grid-cols-3" role="radiogroup" aria-label="When a ticket is claimed">
						{#each claimLocks as c (c.value)}
							{@const selected = form.claim_lock === c.value}
							<button
								type="button"
								role="radio"
								aria-checked={selected}
								disabled={form.mode === 'thread' && c.value !== 'off'}
								onclick={() => (form.claim_lock = c.value)}
								class="rounded-xl border p-4 text-left transition-colors disabled:opacity-50 {selected
									? 'border-accent bg-accent/5'
									: 'border-border hover:border-border-strong'}"
							>
								<span class="block text-sm font-medium">{c.label}</span>
								<span class="mt-0.5 block text-xs text-muted">{c.body}</span>
							</button>
						{/each}
					</div>
					{#if errors.claim_lock}<p class="text-xs text-danger">{errors.claim_lock}</p>{/if}
					{#if form.claim_lock !== 'off'}
						<Field
							label="Roles that keep full access"
							for="claim_exempt"
							optional
							hint={supportRolesOnly.length
								? 'Support roles that can always read and reply, such as senior staff. Administrators always can.'
								: 'Add support roles under Basics first.'}
							error={errors.claim_lock_exempt_role_ids}
						>
							<RolePicker id="claim_exempt" roles={supportRolesOnly} bind:value={form.claim_lock_exempt_role_ids} />
						</Field>
						<p class="hint">The claimer unclaiming, or the ticket moving, gives the team their access back.</p>
					{/if}
				</section>

				<section class="space-y-5 p-5">
					{@render heading(
						'Reply target and reminders',
						"How long a member should wait for the team, measured from their first unanswered message. Tickets on hold don't count."
					)}
					<div class="grid gap-5 sm:grid-cols-2">
						<Field
							label="Reply target"
							for="reply_target"
							hint="Analytics shows how often the first reply beat it, and Home counts tickets past it."
							error={errors.reply_target_minutes}
						>
							<select
								id="reply_target"
								class="input"
								value={form.reply_target_minutes?.toString() ?? ''}
								onchange={(e) => (form.reply_target_minutes = minutesOrNull(e.currentTarget.value))}
								aria-invalid={!!errors.reply_target_minutes}
							>
								{#each minuteChoices as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
							</select>
						</Field>
						<Field
							label="Remind the team"
							for="reminder"
							hint="Posts a reminder once a ticket has waited this long."
							error={errors.reminder_minutes}
						>
							<select
								id="reminder"
								class="input"
								value={form.reminder_minutes?.toString() ?? ''}
								onchange={(e) => (form.reminder_minutes = minutesOrNull(e.currentTarget.value))}
								aria-invalid={!!errors.reminder_minutes}
							>
								{#each reminderChoices as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
							</select>
						</Field>
					</div>
					{#if form.reminder_minutes}
						<div class="grid gap-5 sm:grid-cols-2">
							<Field label="Post reminders" for="reminder_where" error={errors.reminder_where}>
								<select id="reminder_where" class="input" bind:value={form.reminder_where}>
									{#each reminderWheres as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
								</select>
							</Field>
							<Field label="Mention" for="reminder_ping" hint="Only applies to reminders in the ticket." error={errors.reminder_ping}>
								<select id="reminder_ping" class="input" bind:value={form.reminder_ping}>
									{#each reminderPings as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
								</select>
							</Field>
						</div>
						<label class="flex cursor-pointer items-center gap-2 text-sm">
							<input type="checkbox" class="size-4 accent-accent" bind:checked={form.reminder_repeat} />
							Keep reminding every {minutesLabel(form.reminder_minutes)} until someone replies
						</label>
					{/if}
				</section>

				<section class="space-y-5 p-5">
					<Field
						label="Close inactive tickets"
						for="auto_close"
						hint={autoCloseHint}
						help="auto-close"
						error={errors.auto_close_hours}
					>
						<select
							id="auto_close"
							class="input sm:w-56"
							value={form.auto_close_hours?.toString() ?? ''}
							onchange={(e) =>
								(form.auto_close_hours = e.currentTarget.value ? Number(e.currentTarget.value) : null)}
							aria-invalid={!!errors.auto_close_hours}
						>
							{#each autoCloseOptions as o (o.value)}
								<option value={o.value}>{o.label}</option>
							{/each}
						</select>
					</Field>
				</section>
			{:else}
				<section class="space-y-5 p-5">
					{@render heading(
						'When a ticket closes',
						'The member gets a message saying their ticket was closed, with a link to the transcript.'
					)}
					<label class="flex cursor-pointer items-start gap-3 text-sm">
						<input type="checkbox" class="mt-0.5 size-4 accent-accent" bind:checked={form.ask_rating} />
						<span>
							<span class="block font-medium">Ask for a rating</span>
							<span class="mt-0.5 block text-xs text-muted">
								One to five stars, with an optional comment. Ratings show up in Analytics{form.ask_rating
									? ' and in your log channel'
									: ''}. Turn this off for types where it would feel wrong, like reporting someone.
							</span>
						</span>
					</label>
					{#if form.mode === 'channel'}
						<div class="border-t border-border pt-5">
							{@render heading(
								'The ticket channel',
								'What happens to the channel once the ticket is closed and the transcript is saved.'
							)}
						</div>
						<div class="grid gap-3 sm:grid-cols-2" role="radiogroup" aria-label="After closing">
							<button
								type="button"
								role="radio"
								aria-checked={form.closed_parent_id === null}
								onclick={() => (form.closed_parent_id = null)}
								class="rounded-xl border p-4 text-left transition-colors {form.closed_parent_id === null
									? 'border-accent bg-accent/5'
									: 'border-border hover:border-border-strong'}"
							>
								<span class="block text-sm font-medium">Delete the channel</span>
								<span class="mt-0.5 block text-xs text-muted">
									A few seconds after closing. Closed tickets can't be reopened.
								</span>
							</button>
							<button
								type="button"
								role="radio"
								aria-checked={form.closed_parent_id !== null}
								onclick={() => {
									if (form.closed_parent_id === null) form.closed_parent_id = '';
								}}
								class="rounded-xl border p-4 text-left transition-colors {form.closed_parent_id !== null
									? 'border-accent bg-accent/5'
									: 'border-border hover:border-border-strong'}"
							>
								<span class="block text-sm font-medium">Keep it in a category</span>
								<span class="mt-0.5 block text-xs text-muted">
									Read only, for a while, so the ticket can be reopened. Then it's deleted.
								</span>
							</button>
						</div>
						{#if form.closed_parent_id !== null}
							<Field
								label="Closed tickets category"
								for="closed_parent"
								hint="Use a different category from open tickets: Discord allows 50 channels per category."
								error={errors.closed_parent_id}
							>
								<ChannelSelect
									id="closed_parent"
									{channels}
									kinds={['category']}
									bind:value={form.closed_parent_id}
									placeholder="Select a category"
									invalid={!!errors.closed_parent_id}
								/>
							</Field>
							<div class="grid gap-3 sm:grid-cols-2" role="radiogroup" aria-label="Who can see a closed ticket">
								{#each closedAccess as c (c.value)}
									{@const selected = form.closed_member_access === c.value}
									<button
										type="button"
										role="radio"
										aria-checked={selected}
										onclick={() => (form.closed_member_access = c.value)}
										class="rounded-xl border p-4 text-left transition-colors {selected
											? 'border-accent bg-accent/5'
											: 'border-border hover:border-border-strong'}"
									>
										<span class="block text-sm font-medium">{c.label}</span>
										<span class="mt-0.5 block text-xs text-muted">{c.body}</span>
									</button>
								{/each}
							</div>
							<Field
								label="Delete after"
								for="closed_keep_days"
								hint="Reopening is possible until then, from the closing message, the member's DM or the Tickets page."
								error={errors.closed_keep_days}
							>
								<select id="closed_keep_days" class="input sm:w-56" bind:value={form.closed_keep_days}>
									{#each CLOSED_KEEP_DAYS as d (d)}<option value={d}>{keepDaysLabel(d)}</option>{/each}
								</select>
							</Field>
						{/if}
					{/if}
					{#if form.ask_rating}
						<Field
							label="Rating request"
							for="rating_prompt"
							optional
							hint="The question under the closing message. Leave empty for the default."
							error={errors.rating_prompt}
						>
							<textarea
								id="rating_prompt"
								class="input"
								rows="2"
								bind:this={ratingInput}
								bind:value={form.rating_prompt}
								maxlength={MAX_RATING_PROMPT}
								placeholder={DEFAULT_RATING_PROMPT}
								aria-invalid={!!errors.rating_prompt}
							></textarea>
							<PlaceholderChips items={ratingPlaceholders} target={ratingInput} bind:value={form.rating_prompt} />
						</Field>
					{/if}
				</section>
			{/if}
		</div>

		<div
			class="sticky bottom-4 mt-5 flex items-center justify-between gap-2 rounded-xl border border-border bg-surface/90 p-3 shadow-2xl shadow-black/40 backdrop-blur"
		>
			<span class="pl-1 text-xs text-muted">
				{#if initial}{dirty ? 'Unsaved changes' : 'All changes saved'}{:else}Every tab is saved together{/if}
			</span>
			<div class="flex gap-2">
				<a href="/servers/{guildId}/ticket-types" class="btn btn-ghost">{initial ? 'Back' : 'Cancel'}</a>
				<button type="submit" class="btn btn-primary" disabled={saving || (!!initial && !dirty)}>
					{saving ? 'Saving…' : initial ? 'Save changes' : 'Create ticket type'}
				</button>
			</div>
		</div>
	</form>

	<aside class="space-y-6 lg:sticky lg:top-6 lg:self-start">
		<div>
			<p class="mb-3 text-sm text-muted">Its button</p>
			<div class="rounded-xl bg-[#313338] p-4">
				<span
					class="inline-flex items-center gap-1.5 rounded px-4 py-1.5 text-sm font-medium text-white"
					style="background:{buttonSwatch}"
				>
					{#if form.emoji}<span>{form.emoji.replace(/^<a?:(\w+):\d+>$/, ':$1:')}</span>{/if}
					{form.button_label || form.name || 'Ticket type'}
				</span>
			</div>
		</div>
		<div>
			<p class="mb-3 text-sm text-muted">The welcome message in each new ticket</p>
			<WelcomePreview
				name={form.name}
				welcome={form.welcome_message}
				fallback={DEFAULT_WELCOME}
				questions={form.questions}
				server={getGuild()?.name ?? 'your server'}
				supportRoles={form.support_role_ids
					.map((id) => roles.find((r) => r.id === id))
					.filter((r): r is Role => !!r)}
			/>
		</div>
	</aside>
</div>
