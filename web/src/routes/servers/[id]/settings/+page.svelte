<script lang="ts">
	import { getContext, onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import {
		api,
		ApiError,
		atLeast,
		errorMessage,
		MAX_BLOCK_REASON,
		send,
		type Block,
		type Channel,
		type DashboardRole,
		type Guild,
		type GuildSettings,
		type Role
	} from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { timeAgo } from '$lib/format';
	import { toast } from '$lib/toast.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import Dialog from '$lib/components/Dialog.svelte';
	import Field from '$lib/components/Field.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';

	type RoleLevel = DashboardRole['level'];
	const levels: { value: RoleLevel; label: string; body: string }[] = [
		{ value: 'viewer', label: 'Viewer', body: 'Read tickets, transcripts and analytics.' },
		{ value: 'support', label: 'Support', body: 'Viewer, plus reply to, close, reopen, move and hold tickets, add or remove people, and block members.' },
		{ value: 'admin', label: 'Admin', body: 'Support, plus change ticket types, buttons, saved replies and settings.' }
	];

	const getGuild = getContext<() => Guild>('guild');
	const guild = $derived(getGuild());

	const retention = [
		{ value: '', label: 'Keep forever' },
		{ value: '7', label: '7 days' },
		{ value: '30', label: '30 days' },
		{ value: '90', label: '90 days' },
		{ value: '180', label: '6 months' },
		{ value: '365', label: '1 year' }
	];

	let settings = $state<GuildSettings | null>(null);
	let roles = $state<Role[]>([]);
	let channels = $state<Channel[]>([]);
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
	let saving = $state(false);
	let errors = $state<Record<string, string>>({});

	const dirty = $derived(
		settings !== null &&
			(retentionValue !== (settings.transcript_retention_days?.toString() ?? '') ||
				!sameRoles(dashboardRoles, settings.dashboard_roles) ||
				logChannel !== settings.log_channel_id)
	);

	function reset(s: GuildSettings) {
		settings = s;
		retentionValue = s.transcript_retention_days?.toString() ?? '';
		dashboardRoles = s.dashboard_roles.map((r) => ({ ...r }));
		logChannel = s.log_channel_id;
	}

	async function load() {
		loadError = '';
		try {
			const [s, r, c, b] = await Promise.all([
				api<GuildSettings>(`/guilds/${guild.id}/settings`),
				api<Role[]>(`/guilds/${guild.id}/roles`),
				api<Channel[]>(`/guilds/${guild.id}/channels`),
				api<Block[]>(`/guilds/${guild.id}/blocks`)
			]);
			roles = r;
			channels = c;
			blocks = b;
			reset(s);
		} catch (e) {
			loadError = errorMessage(e);
		}
	}

	// --- Blocked members ---
	let blocks = $state<Block[]>([]);
	let blockOpen = $state(false);
	let blockForm = $state({ user_id: '', reason: '' });
	let blockErrors = $state<Record<string, string>>({});
	let blocking = $state(false);
	let unblocking = $state<string | null>(null);

	function openBlock() {
		blockForm = { user_id: '', reason: '' };
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
				send('POST', { user_id: blockForm.user_id.trim(), reason: blockForm.reason })
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
				log_channel_id: logChannel
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
	<div class="mt-6 rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm">
		<p class="text-danger">{loadError}</p>
		<button onclick={load} class="mt-3 text-muted underline-offset-4 hover:text-fg hover:underline">
			Try again
		</button>
	</div>
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
	<form onsubmit={save} class="mt-8 max-w-3xl">
		<div class="divide-y divide-border border-y border-border">
			<section class="grid gap-4 py-6 md:grid-cols-[13rem_minmax(0,1fr)] md:gap-8">
				<div>
					<h2 class="font-medium">Ticket log</h2>
					<p class="hint mt-1">
						A note is posted here whenever a ticket is opened, claimed or closed, so your team can follow
						along in one place.
					</p>
				</div>
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
			</section>

			<section class="grid gap-4 py-6 md:grid-cols-[13rem_minmax(0,1fr)] md:gap-8">
				<div>
					<h2 class="font-medium">Dashboard access</h2>
					<p class="hint mt-1">
						People with Manage Server can always use everything here. Give other roles access at the level
						they need.
						<a href="/help/support-team" target="_blank" class="whitespace-nowrap text-fg underline-offset-4 hover:underline">
							Learn more
						</a>
					</p>
				</div>
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
			</section>

			<section class="grid gap-4 py-6 md:grid-cols-[13rem_minmax(0,1fr)] md:gap-8">
				<div>
					<h2 class="font-medium">Transcripts</h2>
					<p class="hint mt-1">Messages in tickets are saved so you can read them back after a ticket closes.</p>
				</div>
				<Field
					label="Keep transcripts for"
					for="retention"
					hint="Older transcripts are deleted automatically. Ticket details and stats are kept."
					help="transcripts"
					error={errors.transcript_retention_days}
				>
					<select id="retention" class="input sm:w-56" bind:value={retentionValue}>
						{#each retention as o (o.value)}
							<option value={o.value}>{o.label}</option>
						{/each}
					</select>
				</Field>
			</section>
		</div>

		<div
			class="sticky bottom-4 mt-6 flex items-center justify-between gap-2 rounded-xl border border-border bg-surface/90 p-3 shadow-2xl shadow-black/40 backdrop-blur"
		>
			<span class="pl-1 text-xs text-muted">{dirty ? 'Unsaved changes' : 'All changes saved'}</span>
			<button type="submit" class="btn btn-primary" disabled={!dirty || saving || !atLeast(guild.level, 'admin')}>
				{saving ? 'Saving…' : 'Save settings'}
			</button>
		</div>
	</form>

	<section class="mt-10 max-w-3xl border-t border-border grid gap-4 py-6 md:grid-cols-[13rem_minmax(0,1fr)] md:gap-8">
		<div>
			<h2 class="font-medium">Blocked members</h2>
			<p class="hint mt-1">
				People who can't open tickets here, for spam or abuse. Changes apply straight away. Staff can also
				use <code>/ticket block</code> in Discord.
			</p>
		</div>
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
									Blocked by {b.blocked_by_name} {timeAgo(b.created_at)}{b.reason ? `: ${b.reason}` : ''}
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
	</section>

	{#if guild.can_manage}
		<section class="max-w-3xl border-t border-border grid gap-4 py-6 md:grid-cols-[13rem_minmax(0,1fr)] md:gap-8">
			<div>
				<h2 class="font-medium">Delete server data</h2>
				<p class="hint mt-1">
					Removes every ticket and transcript, rating, ticket type, set of ticket buttons, saved reply,
					blocked member and setting stored for this server. The bot stays in the server.
				</p>
			</div>
			<div class="space-y-3">
				<p class="text-sm text-muted">
					This can't be undone. Close any open tickets first. If you remove the bot instead, its data is
					deleted automatically after 30 days.
				</p>
				<button class="btn btn-danger h-8 px-3" onclick={openDelete}>Delete all server data</button>
			</div>
		</section>
	{/if}
{/if}

<Dialog
	bind:open={deleteOpen}
	title="Delete all server data?"
	description="Every ticket, transcript, ticket type, set of ticket buttons, saved reply and setting for this server will be deleted for good."
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
