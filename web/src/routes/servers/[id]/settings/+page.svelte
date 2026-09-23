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
		type Member,
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
	import Segmented from '$lib/components/Segmented.svelte';
	import SettingsSection from '$lib/components/SettingsSection.svelte';

	type RoleLevel = DashboardRole['level'];
	const levels: { value: RoleLevel; label: string; body: string }[] = [
		{ value: 'viewer', label: 'Viewer', body: 'Reads tickets, transcripts and analytics.' },
		{
			value: 'support',
			label: 'Support',
			body: 'Also replies to, closes, reopens, moves and holds tickets, adds or removes people, and blocks members.'
		},
		{ value: 'admin', label: 'Admin', body: 'Also changes ticket types, ticket panels, saved replies and settings.' }
	];

	const getGuild = getContext<() => Guild>('guild');
	const guild = $derived(getGuild());
	const canEdit = $derived(atLeast(guild.level, 'admin'));

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
	let errors = $state<Record<string, string>>({});

	// What the controls show. Each change saves on its own; a failed save puts
	// the control back to what's stored.
	let logChannel = $state<string | null>(null);
	let retentionValue = $state('');
	let dashboardRoles = $state<DashboardRole[]>([]);

	function reset(s: GuildSettings) {
		settings = s;
		logChannel = s.log_channel_id;
		retentionValue = s.transcript_retention_days?.toString() ?? '';
		dashboardRoles = s.dashboard_roles.map((r) => ({ ...r }));
	}

	const unusedRoles = $derived(roles.filter((r) => !dashboardRoles.some((d) => d.role_id === r.id)));
	const roleName = (id: string) => roles.find((r) => r.id === id)?.name ?? 'Deleted role';
	const roleColor = (id: string) => {
		const c = roles.find((r) => r.id === id)?.color;
		return c ? `#${c.toString(16).padStart(6, '0')}` : 'var(--color-subtle)';
	};

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

	// --- Saving as you go ---

	let savedIn = $state('');
	let savedTimer: ReturnType<typeof setTimeout> | undefined;
	let saving = $state('');

	/** Saves part of the settings; `section` shows the "Saved" note. */
	async function patch(section: string, body: Partial<GuildSettings>) {
		if (!settings) return;
		saving = section;
		errors = {};
		try {
			const s = await api<GuildSettings>(`/guilds/${guild.id}/settings`, send('PATCH', body));
			reset(s);
			savedIn = section;
			clearTimeout(savedTimer);
			savedTimer = setTimeout(() => (savedIn = ''), 2500);
		} catch (err) {
			reset(settings);
			if (err instanceof ApiError && err.field) errors = { [err.field]: err.message };
			else toast(errorMessage(err), 'error');
		} finally {
			saving = '';
		}
	}

	function saveLogChannel(id: string | null) {
		// The weekly summary needs a log channel to post to.
		patch('log', id ? { log_channel_id: id } : { log_channel_id: null, weekly_summary: false });
	}

	// Keeping transcripts for less time deletes the older ones, so that asks first.
	let shortenOpen = $state(false);
	let shortenTo = $state('');
	const retentionDays = (v: string) => (v ? Number(v) : Infinity);
	const retentionLabel = (v: string) => allRetention.find((o) => o.value === v)?.label.toLowerCase() ?? v;

	function pickRetention(v: string) {
		const current = settings?.transcript_retention_days?.toString() ?? '';
		if (retentionDays(v) < retentionDays(current)) {
			shortenTo = v;
			shortenOpen = true;
			return;
		}
		patch('transcripts', { transcript_retention_days: v ? Number(v) : null });
	}
	function confirmShorten() {
		shortenOpen = false;
		patch('transcripts', { transcript_retention_days: Number(shortenTo) });
	}
	$effect(() => {
		// Closing the question without confirming leaves the setting as it was.
		if (!shortenOpen && settings) retentionValue = settings.transcript_retention_days?.toString() ?? '';
	});

	function saveRoles(next: DashboardRole[]) {
		dashboardRoles = next;
		patch('access', { dashboard_roles: next });
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
	let blockForm = $state({ reason: '', duration: '' });
	let blockErrors = $state<Record<string, string>>({});
	let blocking = $state(false);
	let unblocking = $state<string | null>(null);

	// Find the member by name (or paste an ID) rather than copying an ID from Discord.
	let who = $state<Member | null>(null);
	let whoQuery = $state('');
	let whoResults = $state<Member[]>([]);
	let whoSearching = $state(false);
	let whoSeq = 0;
	const looksLikeID = $derived(/^\d{15,21}$/.test(whoQuery.trim()));

	$effect(() => {
		const q = whoQuery.trim();
		if (!blockOpen || who || !q) {
			whoResults = [];
			whoSearching = false;
			return;
		}
		const req = ++whoSeq;
		whoSearching = true;
		const timer = setTimeout(async () => {
			try {
				const found = await api<Member[]>(`/guilds/${guild.id}/members?q=${encodeURIComponent(q)}`);
				if (req === whoSeq) whoResults = found;
			} catch {
				if (req === whoSeq) whoResults = [];
			} finally {
				if (req === whoSeq) whoSearching = false;
			}
		}, 250);
		return () => clearTimeout(timer);
	});

	function openBlock() {
		blockForm = { reason: '', duration: '' };
		blockErrors = {};
		who = null;
		whoQuery = '';
		blockOpen = true;
	}

	async function block(e: SubmitEvent) {
		e.preventDefault();
		const userID = who?.id ?? (looksLikeID ? whoQuery.trim() : '');
		if (!userID) return;
		blocking = true;
		blockErrors = {};
		try {
			const created = await api<Block>(
				`/guilds/${guild.id}/blocks`,
				send('POST', { user_id: userID, reason: blockForm.reason, duration: blockForm.duration })
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

	// --- The section index, which follows the scroll ---
	const sections = $derived([
		{ id: 'log', label: 'Ticket log' },
		{ id: 'transcripts', label: 'Transcripts' },
		{ id: 'access', label: 'Dashboard access' },
		{ id: 'blocked', label: 'Blocked members' },
		...(billingEnabled ? [{ id: 'plan', label: 'Plan' }] : []),
		...(guild.can_manage ? [{ id: 'delete', label: 'Delete data' }] : [])
	]);
	let current = $state('log');
	$effect(() => {
		if (!settings) return;
		void sections;
		const seen = new Map<string, boolean>();
		const io = new IntersectionObserver(
			(entries) => {
				for (const e of entries) seen.set(e.target.id, e.isIntersecting);
				const first = sections.find((s) => seen.get(s.id));
				if (first) current = first.id;
			},
			{ rootMargin: '-10% 0px -55% 0px' }
		);
		for (const s of sections) {
			const el = document.getElementById(s.id);
			if (el) io.observe(el);
		}
		return () => io.disconnect();
	});
</script>

<svelte:head><title>Settings · {guild.name} · {APP_NAME}</title></svelte:head>

<PageHeader title="Settings" description="Server-wide options for how tickets are handled. Changes save as you make them." />

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
	<div class="mt-8 lg:grid lg:grid-cols-[10rem_minmax(0,48rem)] lg:gap-12">
		<nav aria-label="Settings sections" class="hidden lg:block">
			<ul class="sticky top-12 space-y-px border-l border-border">
				{#each sections as s (s.id)}
					<li>
						<a
							href="#{s.id}"
							aria-current={current === s.id ? 'true' : undefined}
							class="-ml-px block border-l-2 py-1.5 pl-4 text-sm transition-colors {current === s.id
								? 'border-accent text-fg'
								: 'border-transparent text-muted hover:text-fg'}"
						>
							{s.label}
						</a>
					</li>
				{/each}
			</ul>
		</nav>

		<div class="min-w-0">
			{#if !canEdit}
				<p class="mb-4 rounded-lg border border-border px-4 py-3 text-sm text-muted">
					You can see these settings, but only admins can change them.
				</p>
			{/if}

			<div class="divide-y divide-border border-y border-border">
				<SettingsSection id="log" title="Ticket log" saved={savedIn === 'log'}>
					{#snippet description()}
						A note is posted here whenever a ticket is opened, claimed or closed, so your team can follow
						along in one place.
					{/snippet}
					<div class="space-y-4">
						<Field
							label="Log channel"
							for="log_channel"
							optional
							hint="Use a channel only staff can see. {APP_NAME} needs permission to send messages there."
							help="permissions"
							error={errors.log_channel_id}
						>
							<div class="sm:max-w-sm">
								<ChannelSelect
									id="log_channel"
									{channels}
									kinds={['text', 'announcement']}
									bind:value={logChannel}
									placeholder="Don't log tickets"
									invalid={!!errors.log_channel_id}
									disabled={!canEdit || saving === 'log'}
									onchange={saveLogChannel}
								/>
							</div>
						</Field>
						<label
							class="flex items-start gap-3 text-sm {logChannel && tier === 'premium' && canEdit
								? 'cursor-pointer'
								: 'opacity-60'}"
						>
							<input
								type="checkbox"
								class="mt-0.5 size-4 accent-accent"
								checked={settings.weekly_summary}
								disabled={!logChannel || tier !== 'premium' || !canEdit || saving === 'log'}
								onchange={(e) => patch('log', { weekly_summary: e.currentTarget.checked })}
							/>
							<span>
								Post a weekly summary
								<span class="mt-0.5 block text-xs text-muted">
									Tickets opened and closed, median first response, satisfaction and the busiest day, once a week.
									{tier !== 'premium' ? 'Premium only.' : !logChannel ? 'Choose a log channel first.' : ''}
								</span>
							</span>
						</label>
						{#if errors.weekly_summary}<p class="text-xs text-danger">{errors.weekly_summary}</p>{/if}
					</div>
				</SettingsSection>

				<SettingsSection id="transcripts" title="Transcripts" saved={savedIn === 'transcripts'}>
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
						<select
							id="retention"
							class="input sm:w-56"
							bind:value={retentionValue}
							disabled={!canEdit || saving === 'transcripts'}
							onchange={(e) => pickRetention(e.currentTarget.value)}
						>
							{#each retention as o (o.value)}
								<option value={o.value}>{o.label}</option>
							{/each}
						</select>
					</Field>
				</SettingsSection>

				<SettingsSection id="access" title="Dashboard access" saved={savedIn === 'access'}>
					{#snippet description()}
						People with Manage Server can always use everything here. Give other roles access at the level
						they need.
						<a
							href="/help/support-team"
							target="_blank"
							class="whitespace-nowrap text-fg underline-offset-4 hover:underline"
						>
							Learn more
						</a>
					{/snippet}
					<div class="space-y-4">
						{#if dashboardRoles.length === 0}
							<p class="rounded-lg border border-dashed border-border px-4 py-3 text-sm text-muted">
								Only people with Manage Server can open this dashboard.
							</p>
						{:else}
							<ul class="divide-y divide-border rounded-lg border border-border">
								{#each dashboardRoles as r (r.role_id)}
									<li class="flex flex-wrap items-center gap-3 px-3 py-2.5">
										<span class="size-2.5 shrink-0 rounded-full" style="background:{roleColor(r.role_id)}"></span>
										<span class="min-w-0 flex-1 truncate text-sm font-medium">{roleName(r.role_id)}</span>
										{#if guild.can_manage}
											<Segmented
												options={levels}
												value={r.level}
												label="Access level for {roleName(r.role_id)}"
												onchange={(level) =>
													saveRoles(dashboardRoles.map((x) => (x.role_id === r.role_id ? { ...x, level } : x)))}
											/>
											<button
												type="button"
												class="btn btn-ghost size-8 p-0"
												onclick={() => saveRoles(dashboardRoles.filter((x) => x.role_id !== r.role_id))}
												aria-label="Remove {roleName(r.role_id)}"
												title="Remove"
											>
												<Icon name="x" size={14} />
											</button>
										{:else}
											<span class="text-sm text-muted">{levels.find((l) => l.value === r.level)?.label}</span>
										{/if}
									</li>
								{/each}
							</ul>
						{/if}
						{#if guild.can_manage && unusedRoles.length > 0 && dashboardRoles.length < 10}
							<select
								class="input sm:w-64"
								aria-label="Give a role access"
								value=""
								disabled={saving === 'access'}
								onchange={(e) => {
									const id = e.currentTarget.value;
									e.currentTarget.value = '';
									if (id) saveRoles([...dashboardRoles, { role_id: id, level: 'support' }]);
								}}
							>
								<option value="">Give a role access…</option>
								{#each unusedRoles as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
							</select>
						{/if}
						<dl class="space-y-1.5 text-xs">
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

				<SettingsSection id="blocked" title="Blocked members">
					{#snippet description()}
						People who can't open tickets here, for spam or abuse. Staff can also use <code>/ticket block</code> in
						Discord.
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
												{b.reason || 'No reason given'}
											</p>
											<p class="mt-0.5 text-xs text-subtle">
												Blocked by {b.blocked_by_name}
												{timeAgo(b.created_at)}{b.expires_at ? `, lifts ${timeAgo(b.expires_at)}` : ''}
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
					<SettingsSection id="plan" title="Plan">
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
				<div class="mt-12 rounded-xl border border-danger/30 px-5">
					<SettingsSection id="delete" title="Delete server data" danger>
						{#snippet description()}
							Removes every ticket and transcript, rating, ticket type, ticket panel, saved reply, blocked member and
							setting stored for this server. The bot stays in the server.
						{/snippet}
						<div class="space-y-3">
							<p class="text-sm text-muted">
								This can't be undone. Close any open tickets first. If you remove the bot instead, its data is deleted
								automatically after 30 days.
							</p>
							<button class="btn btn-danger h-8 px-3" onclick={openDelete}>Delete all server data</button>
						</div>
					</SettingsSection>
				</div>
			{/if}
		</div>
	</div>
{/if}

<Dialog
	bind:open={shortenOpen}
	title="Keep transcripts for {retentionLabel(shortenTo)}?"
	description="Transcripts older than {retentionLabel(shortenTo)} are deleted, including ones you have now, and can't be brought back. Ticket details and stats are kept."
>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (shortenOpen = false)}>Cancel</button>
		<button class="btn btn-danger" onclick={confirmShorten}>Delete older transcripts</button>
	{/snippet}
</Dialog>

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
		{#if who}
			<div class="space-y-1.5">
				<p class="label">Member</p>
				<div class="flex items-center gap-3 rounded-lg border border-border px-3 py-2">
					<img src={who.avatar_url} alt="" class="size-7 rounded-full bg-elevated" />
					<span class="min-w-0 flex-1 truncate text-sm font-medium">{who.name}</span>
					<button type="button" class="text-sm text-muted hover:text-fg" onclick={() => (who = null)}>Change</button>
				</div>
				{#if blockErrors.user_id}<p class="text-xs text-danger">{blockErrors.user_id}</p>{/if}
			</div>
		{:else}
			<Field
				label="Member"
				for="block-user"
				hint="Search by name, or paste a user ID for someone who isn't in the server."
				error={blockErrors.user_id}
			>
				<input
					id="block-user"
					class="input"
					bind:value={whoQuery}
					placeholder="Start typing a name…"
					autocomplete="off"
					aria-invalid={!!blockErrors.user_id}
				/>
			</Field>
			{#if whoQuery.trim()}
				{#if whoResults.length}
					<ul class="divide-y divide-border rounded-lg border border-border" aria-busy={whoSearching}>
						{#each whoResults as m (m.id)}
							<li>
								<button
									type="button"
									class="flex w-full items-center gap-3 px-3 py-2 text-left text-sm transition-colors hover:bg-elevated"
									onclick={() => (who = m)}
								>
									<img src={m.avatar_url} alt="" class="size-7 shrink-0 rounded-full bg-elevated" />
									<span class="min-w-0 flex-1 truncate font-medium">{m.name}</span>
									<span class="shrink-0 text-xs text-subtle">Choose</span>
								</button>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="rounded-lg border border-dashed border-border px-4 py-3 text-sm text-muted">
						{whoSearching
							? 'Searching…'
							: looksLikeID
								? "Nobody in the server has that ID. You can still block it, in case they join."
								: 'Nobody in the server matches that.'}
					</p>
				{/if}
			{/if}
		{/if}
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
		<Field label="For" for="block-duration" error={blockErrors.duration}>
			<select id="block-duration" class="input" bind:value={blockForm.duration}>
				{#each blockDurationOptions as o (o.value)}
					<option value={o.value}>{o.label}</option>
				{/each}
			</select>
		</Field>
	</form>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (blockOpen = false)}>Cancel</button>
		<button type="submit" form="block-member" class="btn btn-danger" disabled={blocking || (!who && !looksLikeID)}>
			{blocking ? 'Blocking…' : 'Block member'}
		</button>
	{/snippet}
</Dialog>

<Dialog
	bind:open={welcomeOpen}
	title="Welcome to premium"
	description="Thanks for supporting {APP_NAME}. Here's what's unlocked for this server:"
>
	<ul class="space-y-3">
		{#each [`Up to ${limits?.max_ticket_types ?? 100} ticket types`, `Up to ${limits?.max_panels ?? 50} ticket panels`, `Up to ${limits?.max_saved_replies ?? 200} saved replies`, 'Transcripts kept forever, not just 90 days', `No "Powered by ${APP_NAME}" footer on ticket panels`, 'A weekly summary posted to your ticket log'] as benefit (benefit)}
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
