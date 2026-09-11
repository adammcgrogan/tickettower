<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import {
		api,
		ApiError,
		errorMessage,
		MAX_QUESTIONS,
		send,
		type Channel,
		type QuestionStyle,
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
	import Segmented from './Segmented.svelte';
	import WelcomePreview from './WelcomePreview.svelte';

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
			max_open_per_user: initial?.max_open_per_user ?? 1,
			questions: initial?.questions.map((q) => ({ ...q })) ?? [],
			auto_close_hours: initial?.auto_close_hours ?? null
		}))
	);

	const autoCloseOptions = [
		{ value: '', label: 'Never' },
		{ value: '12', label: 'After 12 hours' },
		{ value: '24', label: 'After 1 day' },
		{ value: '48', label: 'After 2 days' },
		{ value: '72', label: 'After 3 days' },
		{ value: '168', label: 'After 1 week' }
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

<div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,24rem)]">
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
		<div class="flex items-start justify-between gap-4">
			<div>
				<h2 class="font-medium">Questions</h2>
				<p class="hint mt-1">
					Ask members a few questions before their ticket opens. Their answers are posted in the
					welcome message, so your team has the details up front.
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
		<Field
			label="Close inactive tickets"
			for="auto_close"
			hint={autoCloseHint}
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

	<div
		class="sticky bottom-4 flex items-center justify-end gap-2 rounded-xl border border-border bg-surface/90 p-3 shadow-2xl shadow-black/40 backdrop-blur"
	>
		<a href="/servers/{guildId}/ticket-types" class="btn btn-ghost">Cancel</a>
		<button type="submit" class="btn btn-primary" disabled={saving}>
			{saving ? 'Saving…' : initial ? 'Save changes' : 'Create ticket type'}
		</button>
	</div>
</form>

<aside class="lg:sticky lg:top-6 lg:self-start">
	<p class="mb-3 text-sm text-muted">The welcome message in each new ticket</p>
	<WelcomePreview
		name={form.name}
		welcome={form.welcome_message}
		fallback={DEFAULT_WELCOME}
		questions={form.questions}
		supportRoles={form.support_role_ids
			.map((id) => roles.find((r) => r.id === id))
			.filter((r): r is Role => !!r)}
	/>
</aside>
</div>
