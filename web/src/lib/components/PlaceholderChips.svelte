<script lang="ts">
	import { tick } from 'svelte';
	import type { Placeholder } from '$lib/placeholders';

	/** Buttons that insert a placeholder into a text field at the cursor. */
	let {
		items,
		target,
		value = $bindable()
	}: {
		items: Placeholder[];
		target: HTMLInputElement | HTMLTextAreaElement | undefined;
		value: string;
	} = $props();

	async function insert(token: string) {
		const start = target?.selectionStart ?? value.length;
		const end = target?.selectionEnd ?? value.length;
		value = value.slice(0, start) + token + value.slice(end);
		await tick();
		target?.focus();
		target?.setSelectionRange(start + token.length, start + token.length);
	}
</script>

<div class="flex flex-wrap gap-1.5">
	{#each items as p (p.token)}
		<button
			type="button"
			class="inline-flex max-w-full items-center gap-1.5 rounded-md border border-border px-2 py-0.5 text-xs text-muted transition-colors hover:border-border-strong hover:text-fg"
			onclick={() => insert(p.token)}
			aria-label="Insert {p.token}: {p.label}"
		>
			<code class="font-mono text-fg">{p.token}</code>
			<span class="truncate">{p.label}</span>
		</button>
	{/each}
</div>
