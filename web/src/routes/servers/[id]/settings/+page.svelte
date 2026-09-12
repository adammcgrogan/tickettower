<script lang="ts">
	import { getContext, onMount } from 'svelte';
	import {
		api,
		ApiError,
		errorMessage,
		send,
		type Channel,
		type Guild,
		type GuildSettings,
		type Role
	} from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { toast } from '$lib/toast.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import Field from '$lib/components/Field.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';

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
	let dashboardRoles = $state<string[]>([]);
	let logChannel = $state<string | null>(null);
	let saving = $state(false);
	let errors = $state<Record<string, string>>({});

	const sameSet = (a: string[], b: string[]) =>
		a.length === b.length && a.every((v) => b.includes(v));

	const dirty = $derived(
		settings !== null &&
			(retentionValue !== (settings.transcript_retention_days?.toString() ?? '') ||
				!sameSet(dashboardRoles, settings.dashboard_role_ids) ||
				logChannel !== settings.log_channel_id)
	);

	function reset(s: GuildSettings) {
		settings = s;
		retentionValue = s.transcript_retention_days?.toString() ?? '';
		dashboardRoles = [...s.dashboard_role_ids];
		logChannel = s.log_channel_id;
	}

	async function load() {
		loadError = '';
		try {
			const [s, r, c] = await Promise.all([
				api<GuildSettings>(`/guilds/${guild.id}/settings`),
				api<Role[]>(`/guilds/${guild.id}/roles`),
				api<Channel[]>(`/guilds/${guild.id}/channels`)
			]);
			roles = r;
			channels = c;
			reset(s);
		} catch (e) {
			loadError = errorMessage(e);
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
			if (guild.can_manage) body.dashboard_role_ids = dashboardRoles;
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
	<div class="mt-8 max-w-3xl space-y-5" aria-busy="true">
		{#each Array(2) as _, i (i)}
			<div class="card space-y-4 p-5">
				<div class="h-4 w-32 animate-pulse rounded bg-elevated"></div>
				<div class="h-3 w-2/3 animate-pulse rounded bg-elevated"></div>
				<div class="h-9 w-56 animate-pulse rounded-lg bg-elevated"></div>
			</div>
		{/each}
	</div>
{:else}
	<form onsubmit={save} class="mt-8 max-w-3xl space-y-5">
		<section class="card space-y-5 p-5">
			<div>
				<h2 class="font-medium">Ticket log</h2>
				<p class="hint mt-1">
					{APP_NAME} posts a short note here whenever a ticket is opened, claimed or closed, so your
					team can follow along in one place.
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
				<div class="sm:max-w-sm">
					<ChannelSelect
						id="log_channel"
						{channels}
						kinds={['text', 'announcement']}
						bind:value={logChannel}
						placeholder="Don't log tickets"
						invalid={!!errors.log_channel_id}
					/>
				</div>
			</Field>
		</section>

		<section class="card space-y-5 p-5">
			<div>
				<h2 class="font-medium">Dashboard access</h2>
				<p class="hint mt-1">
					People with Manage Server can always use this dashboard. Add roles to let other members,
					such as your support leads, manage tickets and ticket buttons here too.
				</p>
			</div>
			<Field
				label="Dashboard roles"
				for="dashboard_roles"
				optional
				hint={guild.can_manage
					? 'Members with these roles can use everything except this setting.'
					: 'Only people with Manage Server can change who has access.'}
				help="support-team"
				error={errors.dashboard_role_ids}
			>
				<div class="sm:max-w-sm">
					<RolePicker
						id="dashboard_roles"
						{roles}
						bind:value={dashboardRoles}
						disabled={!guild.can_manage}
					/>
				</div>
				{#if !guild.can_manage && dashboardRoles.length === 0}
					<p class="text-sm text-muted">No roles yet.</p>
				{/if}
			</Field>
		</section>

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

		<div class="flex justify-end">
			<button type="submit" class="btn btn-primary" disabled={!dirty || saving}>
				{saving ? 'Saving…' : 'Save settings'}
			</button>
		</div>
	</form>
{/if}
