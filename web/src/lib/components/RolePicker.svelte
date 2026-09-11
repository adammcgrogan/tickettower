<script lang="ts">
	import type { Role } from '$lib/api';
	import { intToHex } from '$lib/api';
	import Icon from './Icon.svelte';

	let {
		roles,
		value = $bindable([]),
		id,
		max = 10,
		disabled = false
	}: { roles: Role[]; value: string[]; id?: string; max?: number; disabled?: boolean } = $props();

	const selected = $derived(
		value.map((v) => roles.find((r) => r.id === v)).filter((r): r is Role => !!r)
	);
	const available = $derived(roles.filter((r) => !value.includes(r.id)));

	const dot = (r: Role) => (r.color ? intToHex(r.color) : 'var(--color-subtle)');
</script>

<div class="space-y-2">
	{#if selected.length > 0}
		<ul class="flex flex-wrap gap-1.5">
			{#each selected as role (role.id)}
				<li
					class="inline-flex items-center gap-1.5 rounded-md border border-border bg-elevated py-1 pr-1 pl-2 text-xs"
				>
					<span class="size-2 rounded-full" style="background:{dot(role)}"></span>
					{role.name}
					{#if !disabled}
						<button
							type="button"
							onclick={() => (value = value.filter((v) => v !== role.id))}
							class="rounded p-0.5 text-subtle transition-colors hover:bg-border hover:text-fg"
							aria-label="Remove {role.name}"
						>
							<Icon name="x" size={12} />
						</button>
					{:else}
						<span class="w-1"></span>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}

	{#if value.length < max && !disabled}
		<select
			{id}
			class="input"
			value=""
			onchange={(e) => {
				const v = e.currentTarget.value;
				if (v) value = [...value, v];
				e.currentTarget.value = '';
			}}
		>
			<option value="">{available.length ? 'Add a role…' : 'No more roles to add'}</option>
			{#each available as role (role.id)}
				<option value={role.id}>{role.name}</option>
			{/each}
		</select>
	{/if}
</div>
