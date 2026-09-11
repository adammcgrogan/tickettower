<script lang="ts">
	import type { Channel } from '$lib/api';

	let {
		channels,
		value = $bindable(null),
		kinds,
		placeholder = 'Select a channel',
		id,
		invalid = false
	}: {
		channels: Channel[];
		value: string | null;
		kinds: Channel['kind'][];
		placeholder?: string;
		id?: string;
		invalid?: boolean;
	} = $props();

	const prefix = (c: Channel) => (c.kind === 'category' ? '' : '# ');

	// Group selectable channels under their categories, as Discord does.
	const groups = $derived.by(() => {
		const matching = channels.filter((c) => kinds.includes(c.kind));
		if (kinds.length === 1 && kinds[0] === 'category') return [{ label: '', items: matching }];

		const categories = channels.filter((c) => c.kind === 'category');
		const out = [{ label: '', items: matching.filter((c) => !c.parent_id) }];
		for (const cat of categories) {
			const items = matching.filter((c) => c.parent_id === cat.id);
			if (items.length) out.push({ label: cat.name, items });
		}
		return out.filter((g) => g.items.length);
	});
</script>

<select
	{id}
	class="input"
	aria-invalid={invalid}
	value={value ?? ''}
	onchange={(e) => (value = e.currentTarget.value || null)}
>
	<option value="">{placeholder}</option>
	{#each groups as group (group.label)}
		{#if group.label}
			<optgroup label={group.label}>
				{#each group.items as c (c.id)}
					<option value={c.id}>{prefix(c)}{c.name}</option>
				{/each}
			</optgroup>
		{:else}
			{#each group.items as c (c.id)}
				<option value={c.id}>{prefix(c)}{c.name}</option>
			{/each}
		{/if}
	{/each}
</select>
