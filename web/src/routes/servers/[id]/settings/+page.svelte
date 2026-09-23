<script lang="ts">
	import { getContext, onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		api,
		ApiError,
		atLeast,
		errorMessage,
		blockDurationOptions,
		MAX_BLOCK_REASON,
		send,
		type Block,
		type Channel,
		type DashboardRole,
		type Guild,
		type GuildSettings,
		type Role,
		type Stats
	} from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { timeAgo } from '$lib/format';
	import { toast } from '$lib/toast.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import Dialog from '$lib/components/Dialog.svelte';
	import Field from '$lib/components/Field.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import LoadError from '$lib/components/LoadError.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import SaveBar from '$lib/components/SaveBar.svelte';
	import SettingsSection from '$lib/components/SettingsSection.svelte';

	type RoleLevel = DashboardRole['level'];
	const levels: { value: RoleLevel; label: string; body: string }[] = [
		{ value: 'viewer', label: 'Viewer', body: 'Read tickets, transcripts and analytics.' },
		{ value: 'support', label: 'Support', body: 'Viewer, plus reply to, close, reopen, move and hold tickets, add or remove people, and block members.' },
		{ value: 'admin', label: 'Admin', body: 'Support, plus change ticket types, ticket panels, saved replies and settings.' }
	];

	const getGuild = getContext<() => Guild>('guild');
	const guild = $derived(getGuild());

	const allRetention = [
		{ value: '', label: 'Keep forever', days: Infinity },
		{ value: '7', label: '7 days', days: 7 },
		{ value: '30', label: '30 days', days: 30 },
		{ value: '90', label: '90 days', days: 90 },
		{ value: '180', label: '6 months', days: 180 },
		{ value: '365', label: '1 year', days: 365 }
	];

	let settings = $state<GuildSettings | null>(null);
	let limits = $state<Stats['limits'] | null>(null);
	let tier = $state<Stats['tier']>('free');
	let hasBillingCustomer = $state(false);
	let billingEnabled = $state(false);
	let roles = $state<Role[]>([]);
	let channels = $state<Channel[]>([]);
	const retentionCap = $derived(limits?.max_transcript_retention_days || Infinity);
	const retention = $derived(allRetention.filter((o) => o.days <= retentionCap));
	let loadError = $state('');
	let retentionValue = $state('');
	let dashboardRoles = $state<DashboardRole[]>([]);
	let addRole = $state('');

	const sameRoles = (a: DashboardRole[], b: DashboardRole[]) =>
		a.length === b.length && a.every((r) => b.some((o) => o.role_id === r.role_id && o.level === r.level));
	const unusedRoles = $derived(roles.filter((r) => !dashboardRoles.some((d) => d.role_id === r.id)));
	const roleName = (id: string) => roles.find((r) => r.id === id)?.name ?? 'Deleted role';

	function addDashboardRole() {
		if (!addRole) return;
		dashboardRoles = [...dashboardRoles, { role_id: addRole, level: 'support' }];
		addRole = '';
	}
	let logChannel = $state<string | null>(null);
	let weeklySummary = $state(false);
	let saving = $state(false);
	let errors = $state<Record<string, string>>({});

	const dirty = $derived(
		settings !== null &&
			(retentionValue !== (settings.transcript_retention_days?.toString() ?? '') ||
				!sameRoles(dashboardRoles, settings.dashboard_roles) ||
				logChannel !== settings.log_channel_id ||
				weeklySummary !== settings.weekly_summary)
	);

	function reset(s: GuildSettings) {
		settings = s;
		retentionValue = s.transcript_retention_days?.toString() ?? '';
		dashboardRoles = s.dashboard_roles.map((r) => ({ ...r }));
		logChannel = s.log_channel_id;
		weeklySummary = s.weekly_summary;
	}

	// The weekly summary needs a log channel to post to.
	$effect(() => {
		if (!logChannel) weeklySummary = false;
	});

	async function load() {
		loadError = '';
		try {
			const [s, r, c, b, st] = await Promise.all([
				api<GuildSettings>(`/guilds/${guild.id}/settings`),
				api<Role[]>(`/guilds/${guild.id}/roles`),
				api<Channel[]>(`/guilds/${guild.id}/channels`),
				api<Block[]>(`/guilds/${guild.id}/blocks`),
				api<Stats>(`/guilds/${guild.id}/stats`)
			]);
			roles = r;
			channels = c;
			blocks = b;
			limits = st.limits;
			tier = st.tier;
			hasBillingCustomer = st.has_billing_customer;
			billingEnabled = st.billing_enabled;
			reset(s);
			checkBillingRedirect();
		} catch (e) {
			loadError = errorMessage(e);
		}
	}

	function checkBillingRedirect() {
		const billing = page.url.searchParams.get('billing');
		if (!billing) return;
		// history.replaceState, not goto: goto re-enters SvelteKit's router for
		// what looks like a fresh navigation to this route, which can reset the
		// dialog's open state before it ever renders. This just edits the URL.
		const url = new URL(page.url);
		url.searchParams.delete('billing');
		history.replaceState(history.state, '', url);
		if (billing === 'success') welcomeOpen = true;
		else if (billing === 'cancel') toast('Checkout was cancelled.');
	}

	// --- Billing ---
	let checkingOut = $state(false);
	let openingPortal = $state(false);
	let welcomeOpen = $state(false);

	async function upgrade() {
		checkingOut = true;
		try {
			const { url } = await api<{ url: string }>(`/guilds/${guild.id}/billing/checkout`, send('POST'));
			window.location.href = url;
		} catch (err) {
			toast(errorMessage(err), 'error');
			checkingOut = false;
		}
	}

	async function manageBilling() {
		openingPortal = true;
		try {
			const { url } = await api<{ url: string }>(`/guilds/${guild.id}/billing/portal`, send('POST'));
			window.location.href = url;
		} catch (err) {
			toast(errorMessage(err), 'error');
			openingPortal = false;
		}
	}

	// --- Blocked members ---
	let blocks = $state<Block[]>([]);
	let blockOpen = $state(false);
	let blockForm = $state({ user_id: '', reason: '', duration: '' });
	let blockErrors = $state<Record<string, string>>({});
	let blocking = $state(false);
	let unblocking = $state<string | null>(null);

	function openBlock() {
		blockForm = { user_id: '', reason: '', duration: '' };
		blockErrors = {};
		blockOpen = true;
	}

	async function block(e: SubmitEvent) {
		e.preventDefault();
		blocking = true;
		blockErrors = {};
		try {
			const created = await api<Block>(
				`/guilds/${guild.id}/blocks`,
				send('POST', {
					user_id: blockForm.user_id.trim(),
					reason: blockForm.reason,
					duration: blockForm.duration
				})
			);
			blocks = [created, ...blocks.filter((b) => b.user_id !== created.user_id)];
			blockOpen = false;
			toast(`${created.user_name} can no longer open tickets`);
		} catch (err) {
			if (err instanceof ApiError && err.field) blockErrors = { [err.field]: err.message };
			else blockErrors = { other: errorMessage(err) };
		} finally {
			blocking = false;
		}
	}

	async function unblock(b: Block) {
		unblocking = b.user_id;
		try {
			await api(`/guilds/${guild.id}/blocks/${b.user_id}`, send('DELETE'));
			blocks = blocks.filter((x) => x.user_id !== b.user_id);
			toast(`${b.user_name} can open tickets again`);
		} catch (err) {
			toast(errorMessage(err), 'error');
		} finally {
			unblocking = null;
		}
	}

	// --- Delete server data ---
	let deleteOpen = $state(false);
	let deleteConfirm = $state('');
	let deleteError = $state('');
	let deleting = $state(false);

	function openDelete() {
		deleteConfirm = '';
		deleteError = '';
		deleteOpen = true;
	}

	async function deleteData(e: SubmitEvent) {
		e.preventDefault();
		if (deleteConfirm.trim() !== guild.name) return;
		deleting = true;
		deleteError = '';
		try {
			await api(`/guilds/${guild.id}/data`, send('DELETE'));
			deleteOpen = false;
			toast('All server data has been deleted');
			await goto(`/servers/${guild.id}`, { invalidateAll: true });
		} catch (err) {
			deleteError = errorMessage(err);
		} finally {
			deleting = false;
		}
	}

	onMount(load);

	async function save(e: SubmitEvent) {
		e.preventDefault();
		saving = true;
		errors = {};
		try {
			const body: Partial<GuildSettings> = {
				transcript_retention_days: retentionValue ? Number(retentionValue) : null,
				log_channel_id: logChannel,
				weekly_summary: weeklySummary
			};
			if (guild.can_manage) body.dashboard_roles = dashboardRoles;
			reset(await api<GuildSettings>(`/guilds/${guild.id}/settings`, send('PATCH', body)));
			toast('Settings saved');
		} catch (err) {
			if (err instanceof ApiError && err.field) errors = { [err.field]: err.message };
			else toast(errorMessage(err), 'error');
		} finally {
			saving = false;
		}
	}
</script>

<PageHeader title="Settings" description="Server-wide options for how tickets are handled." />

{#if loadError}
	<div class="mt-8 max-w-3xl"><LoadError message={loadError} onretry={load} /></div>
{:else if !settings}
	<div class="mt-8 max-w-3xl divide-y divide-border border-y border-border" aria-busy="true">
		{#each Array(3) as _, i (i)}
			<div class="space-y-4 py-6">
				<div class="h-4 w-32 animate-pulse rounded bg-elevated"></div>
				<div class="h-3 w-2/3 animate-pulse rounded bg-elevated"></div>
				<div class="h-9 w-56 animate-pulse rounded-lg bg-elevated"></div>
			</div>
		{/each}
	</div>
{:else}
	<!-- Saved together first, then the parts that apply straight away, then the plan and the danger zone. -->
	<div class="mt-8 max-w-3xl">
	<form onsubmit={save}>
		<div class="divide-y divide-border border-y border-border">
			<SettingsSection title="Ticket log">
				{#snippet description()}
					A note is posted here whenever a ticket is opened, claimed or closed, so your team can follow
					along in one place.
				{/snippet}
				<Field
					label="Log channel"
					for="log_channel"
					optional
					hint="Use a channel only staff can see. {APP_NAME} needs permission to send messages there."
					help="permissions"
					error={errors.log_channel_id}
				>
					<ChannelSelect
						id="log_channel"
						{channels}
						kinds={['text', 'announcement']}
						bind:value={logChannel}
						placeholder="Don't log tickets"
						invalid={!!errors.log_channel_id}
					/>
				</Field>
				{#if logChannel}
					<label class="flex cursor-pointer items-start gap-2 text-sm" class:opacity-60={tier !== 'premium'}>
						<input
							type="checkbox"
							class="mt-0.5 size-4 accent-accent"
							bind:checked={weeklySummary}
							disabled={tier !== 'premium'}
						/>
						<span>
							Post a weekly summary
							<span class="block text-xs text-muted">
								Tickets opened and closed, median first response, satisfaction and the busiest day, once a week.
								{tier === 'premium' ? 'Off by default.' : 'Premium only.'}
							</span>
						</span>
					</label>
					{#if errors.weekly_summary}<p class="text-xs text-danger">{errors.weekly_summary}</p>{/if}
				{/if}
			</SettingsSection>

			<SettingsSection title="Dashboard access">
				{#snippet description()}
					People with Manage Server can always use everything here. Give other roles access at the level
					they need.
					<a href="/help/support-team" target="_blank" class="whitespace-nowrap text-fg underline-offset-4 hover:underline">
						Learn more
					</a>
				{/snippet}
				<div class="space-y-4">
					{#if dashboardRoles.length === 0}
						<p class="rounded-lg border border-dashed border-border px-4 py-3 text-sm text-muted">
							No roles have dashboard access yet.
						</p>
					{:else}
						<ul class="divide-y divide-border rounded-lg border border-border">
							{#each dashboardRoles as r, i (r.role_id)}
								<li class="flex flex-wrap items-center gap-3 px-3 py-2">
									<span class="min-w-0 flex-1 truncate text-sm font-medium">{roleName(r.role_id)}</span>
									<select
										class="input h-8 w-auto"
										aria-label="Access level for {roleName(r.role_id)}"
										bind:value={r.level}
										disabled={!guild.can_manage}
									>
										{#each levels as l (l.value)}<option value={l.value}>{l.label}</option>{/each}
									</select>
									{#if guild.can_manage}
										<button
											type="button"
											class="btn btn-ghost size-8 p-0"
											onclick={() => dashboardRoles.splice(i, 1)}
											aria-label="Remove {roleName(r.role_id)}"
										>
											<Icon name="x" size={14} />
										</button>
									{/if}
								</li>
							{/each}
						</ul>
					{/if}
					{#if guild.can_manage && unusedRoles.length > 0 && dashboardRoles.length < 10}
						<div class="flex flex-wrap items-center gap-2">
							<select class="input w-auto" aria-label="Role to add" bind:value={addRole}>
								<option value="">Add a role…</option>
								{#each unusedRoles as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
							</select>
							<button type="button" class="btn btn-secondary h-9" onclick={addDashboardRole} disabled={!addRole}>
								<Icon name="plus" size={14} /> Add
							</button>
						</div>
					{/if}
					<dl class="space-y-2 text-xs">
						{#each levels as l (l.value)}
							<div class="grid grid-cols-[4.5rem_minmax(0,1fr)] gap-2">
								<dt class="font-medium text-fg">{l.label}</dt>
								<dd class="text-muted">{l.body}</dd>
							</div>
						{/each}
					</dl>
					{#if !guild.can_manage}
						<p class="text-xs text-subtle">Only people with Manage Server can change who has access.</p>
					{/if}
					{#if errors.dashboard_roles}<p class="text-xs text-danger">{errors.dashboard_roles}</p>{/if}
				</div>
			</SettingsSection>

			<SettingsSection title="Transcripts">
				{#snippet description()}
					Messages in tickets are saved so you can read them back after a ticket closes.
				{/snippet}
				<Field
					label="Keep transcripts for"
					for="retention"
					hint={retentionCap === Infinity
						? 'Older transcripts are deleted automatically. Ticket details and stats are kept.'
						: `Older transcripts are deleted automatically. Free plans keep them for up to ${retentionCap} days.`}
					help="transcripts"
					error={errors.transcript_retention_days}
				>
					<select id="retention" class="input sm:w-56" bind:value={retentionValue}>
						{#each retention as o (o.value)}
							<option value={o.value}>{o.label}</option>
						{/each}
					</select>
				</Field>
			</SettingsSection>
		</div>

		<SaveBar status={dirty ? 'Unsaved changes' : 'All changes saved'}>
			<button type="submit" class="btn btn-primary" disabled={!dirty || saving || !atLeast(guild.level, 'admin')}>
				{saving ? 'Saving…' : 'Save settings'}
			</button>
		</SaveBar>
	</form>

	<div class="mt-10 divide-y divide-border border-y border-border">
	<SettingsSection title="Blocked members">
		{#snippet description()}
			People who can't open tickets here, for spam or abuse. Changes apply straight away. Staff can also
			use <code>/ticket block</code> in Discord.
		{/snippet}
		<div class="space-y-4">
			{#if blocks.length === 0}
				<p class="rounded-lg border border-dashed border-border px-4 py-3 text-sm text-muted">Nobody is blocked.</p>
			{:else}
				<ul class="divide-y divide-border rounded-lg border border-border">
					{#each blocks as b (b.user_id)}
						<li class="flex items-start gap-4 px-3 py-2.5">
							<div class="min-w-0 flex-1">
								<p class="truncate text-sm font-medium">{b.user_name}</p>
								<p class="mt-0.5 text-sm text-muted">
									Blocked by {b.blocked_by_name} {timeAgo(b.created_at)}{b.expires_at
										? `, lifts ${timeAgo(b.expires_at)}`
										: ''}{b.reason ? `: ${b.reason}` : ''}
								</p>
							</div>
							<button
								class="btn btn-ghost h-8 shrink-0 px-3"
								onclick={() => unblock(b)}
								disabled={unblocking === b.user_id}
							>
								{unblocking === b.user_id ? 'Unblocking…' : 'Unblock'}
							</button>
						</li>
					{/each}
				</ul>
			{/if}
			<button class="btn btn-secondary h-8 px-3" onclick={openBlock}>
				<Icon name="plus" size={14} /> Block a member
			</button>
		</div>
	</SettingsSection>

	{#if billingEnabled}
		<SettingsSection title="Plan">
			{#snippet description()}
				{tier === 'premium'
					? 'This server is on the premium plan.'
					: `Free plans get up to ${limits?.max_ticket_types ?? 25} ticket types and ${limits?.max_panels ?? 10} ticket panels.`}
			{/snippet}
			{#if tier === 'premium'}
				{#if hasBillingCustomer}
					<button class="btn btn-secondary" onclick={manageBilling} disabled={openingPortal}>
						{openingPortal ? 'Opening…' : 'Manage billing'}
					</button>
				{:else}
					<p class="text-sm text-muted">Premium was granted by the {APP_NAME} team.</p>
				{/if}
			{:else if guild.can_manage}
				<button class="btn btn-primary" onclick={upgrade} disabled={checkingOut}>
					{checkingOut ? 'Starting checkout…' : 'Upgrade to premium'}
				</button>
			{:else}
				<p class="text-sm text-subtle">Only someone with Manage Server can upgrade this server.</p>
			{/if}
		</SettingsSection>
	{/if}
	</div>

	{#if guild.can_manage}
		<div class="mt-10 rounded-xl border border-danger/30 px-5">
			<SettingsSection title="Delete server data" danger>
				{#snippet description()}
					Removes every ticket and transcript, rating, ticket type, ticket panel, saved reply,
					blocked member and setting stored for this server. The bot stays in the server.
				{/snippet}
				<div class="space-y-3">
					<p class="text-sm text-muted">
						This can't be undone. Close any open tickets first. If you remove the bot instead, its data is
						deleted automatically after 30 days.
					</p>
					<button class="btn btn-danger h-8 px-3" onclick={openDelete}>Delete all server data</button>
				</div>
			</SettingsSection>
		</div>
	{/if}
	</div>
{/if}

<Dialog
	bind:open={deleteOpen}
	title="Delete all server data?"
	description="Every ticket, transcript, ticket type, ticket panel, saved reply and setting for this server will be deleted for good."
>
	<form id="delete-data" onsubmit={deleteData} class="space-y-4">
		{#if deleteError}<p class="text-sm text-danger">{deleteError}</p>{/if}
		<Field label={`Type ${guild.name} to confirm`} for="delete-confirm">
			<input
				id="delete-confirm"
				class="input"
				bind:value={deleteConfirm}
				placeholder={guild.name}
				autocomplete="off"
				required
			/>
		</Field>
	</form>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (deleteOpen = false)}>Cancel</button>
		<button
			type="submit"
			form="delete-data"
			class="btn btn-danger"
			disabled={deleting || deleteConfirm.trim() !== guild.name}
		>
			{deleting ? 'Deleting…' : 'Delete everything'}
		</button>
	{/snippet}
</Dialog>

<Dialog
	bind:open={blockOpen}
	title="Block a member"
	description="They won't be able to open any ticket until they're unblocked. Tickets they already have stay open."
>
	<form id="block-member" onsubmit={block} class="space-y-4">
		{#if blockErrors.other}<p class="text-sm text-danger">{blockErrors.other}</p>{/if}
		<Field
			label="User ID"
			for="block-user"
			hint="In Discord, right-click the member and choose Copy User ID. You may need Developer Mode on in Advanced settings."
			error={blockErrors.user_id}
		>
			<input
				id="block-user"
				class="input font-mono text-[13px]"
				inputmode="numeric"
				pattern="[0-9]*"
				bind:value={blockForm.user_id}
				placeholder="e.g. 123456789012345678"
				aria-invalid={!!blockErrors.user_id}
				required
			/>
		</Field>
		<Field
			label="Reason"
			for="block-reason"
			optional
			hint="Shown to the member if they try to open a ticket."
			error={blockErrors.reason}
		>
			<input
				id="block-reason"
				class="input"
				maxlength={MAX_BLOCK_REASON}
				bind:value={blockForm.reason}
				placeholder="e.g. Repeated spam tickets"
				aria-invalid={!!blockErrors.reason}
			/>
		</Field>
		<Field label="Duration" for="block-duration" error={blockErrors.duration}>
			<select id="block-duration" class="input" bind:value={blockForm.duration}>
				{#each blockDurationOptions as o (o.value)}
					<option value={o.value}>{o.label}</option>
				{/each}
			</select>
		</Field>
	</form>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (blockOpen = false)}>Cancel</button>
		<button
			type="submit"
			form="block-member"
			class="btn btn-danger"
			disabled={blocking || !blockForm.user_id.trim()}
		>
			{blocking ? 'Blocking…' : 'Block member'}
		</button>
	{/snippet}
</Dialog>

<Dialog bind:open={welcomeOpen} title="Welcome to premium" description="Thanks for supporting {APP_NAME}. Here's what's unlocked for this server:">
	<ul class="space-y-3">
		{#each [
			`Up to ${limits?.max_ticket_types ?? 100} ticket types`,
			`Up to ${limits?.max_panels ?? 50} ticket panels`,
			`Up to ${limits?.max_saved_replies ?? 200} saved replies`,
			'Transcripts kept forever, not just 90 days',
			`No "Powered by ${APP_NAME}" footer on ticket panels`,
			'A weekly summary posted to your ticket log'
		] as benefit (benefit)}
			<li class="flex items-start gap-3 text-sm">
				<span class="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-accent/15 text-accent-ink">
					<Icon name="check" size={12} />
				</span>
				{benefit}
			</li>
		{/each}
	</ul>
	{#snippet footer()}
		<button class="btn btn-primary" onclick={() => (welcomeOpen = false)}>Let's go</button>
	{/snippet}
</Dialog>
