<script lang="ts" generics="T extends string">
	import Icon, { type IconName } from './Icon.svelte';

	// A radio group drawn as cards, for choices that need a sentence each.
	let {
		options,
		value = $bindable(),
		label,
		columns = 2
	}: {
		options: { value: T; label: string; body: string; icon?: IconName; disabled?: boolean }[];
		value: T;
		label: string;
		columns?: 2 | 3;
	} = $props();
</script>

<div
	class="grid gap-3 {columns === 3 ? 'sm:grid-cols-3' : 'sm:grid-cols-2'}"
	role="radiogroup"
	aria-label={label}
>
	{#each options as o (o.value)}
		{@const selected = value === o.value}
		<button
			type="button"
			role="radio"
			aria-checked={selected}
			disabled={o.disabled}
			onclick={() => (value = o.value)}
			class="flex gap-3 rounded-xl border p-4 text-left transition-colors disabled:opacity-50 {selected
				? 'border-accent bg-accent/5'
				: 'border-border hover:border-border-strong'}"
		>
			{#if o.icon}
				<span
					class="grid size-8 shrink-0 place-items-center rounded-lg bg-elevated {selected
						? 'text-accent-ink'
						: 'text-muted'}"
				>
					<Icon name={o.icon} />
				</span>
			{/if}
			<span>
				<span class="block text-sm font-medium">{o.label}</span>
				<span class="mt-0.5 block text-xs text-muted">{o.body}</span>
			</span>
		</button>
	{/each}
</div>
