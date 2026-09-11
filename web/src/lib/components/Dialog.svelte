<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		open = $bindable(false),
		title,
		description,
		children,
		footer
	}: {
		open?: boolean;
		title: string;
		description?: string;
		children?: Snippet;
		footer?: Snippet;
	} = $props();

	let dialog = $state<HTMLDialogElement>();

	$effect(() => {
		if (!dialog) return;
		if (open && !dialog.open) dialog.showModal();
		else if (!open && dialog.open) dialog.close();
	});
</script>

<dialog
	bind:this={dialog}
	onclose={() => (open = false)}
	onclick={(e) => {
		if (e.target === dialog) open = false;
	}}
	class="m-auto w-[min(100%-2rem,30rem)] rounded-2xl border border-border bg-surface p-0 text-fg shadow-2xl shadow-black/60 backdrop:bg-black/60 backdrop:backdrop-blur-sm"
>
	{#if open}
		<div class="p-6">
			<h2 class="text-base font-semibold">{title}</h2>
			{#if description}
				<p class="mt-1 text-sm text-muted">{description}</p>
			{/if}
			{#if children}
				<div class="mt-5">{@render children()}</div>
			{/if}
		</div>
		{#if footer}
			<div class="flex justify-end gap-2 border-t border-border px-6 py-4">{@render footer()}</div>
		{/if}
	{/if}
</dialog>
