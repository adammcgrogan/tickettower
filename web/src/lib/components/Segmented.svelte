<script lang="ts" generics="T extends string">
	let {
		options,
		value = $bindable(),
		label,
		onchange
	}: {
		options: { value: T; label: string }[];
		value: T;
		label: string;
		onchange?: (value: T) => void;
	} = $props();
</script>

<div role="radiogroup" aria-label={label} class="inline-flex max-w-full overflow-x-auto rounded-lg border border-border bg-surface p-0.5">
	{#each options as o (o.value)}
		<button
			type="button"
			role="radio"
			aria-checked={value === o.value}
			onclick={() => {
				value = o.value;
				onchange?.(o.value);
			}}
			class="rounded-md px-3 py-1.5 text-sm whitespace-nowrap transition-colors {value === o.value
				? 'bg-elevated text-fg shadow-sm'
				: 'text-muted hover:text-fg'}"
		>
			{o.label}
		</button>
	{/each}
</div>
