<script lang="ts">
	import { fly } from 'svelte/transition';
	import { dismiss, toasts } from '$lib/toast.svelte';
	import Icon from './Icon.svelte';
</script>

<div
	class="pointer-events-none fixed right-0 bottom-0 z-50 flex w-full max-w-sm flex-col gap-2 p-4"
	aria-live="polite"
>
	{#each toasts as t (t.id)}
		<div
			transition:fly={{ y: 12, duration: 180 }}
			class="pointer-events-auto flex items-start gap-3 rounded-xl border bg-surface px-4 py-3 text-sm shadow-2xl shadow-black/50 {t.kind ===
			'error'
				? 'border-danger/40'
				: 'border-border'}"
			role={t.kind === 'error' ? 'alert' : 'status'}
		>
			<span
				class="mt-0.5 size-2 shrink-0 rounded-full {t.kind === 'error'
					? 'bg-danger'
					: t.kind === 'success'
						? 'bg-success'
						: 'bg-accent'}"
			></span>
			<p class="flex-1 leading-snug">{t.message}</p>
			<button
				onclick={() => dismiss(t.id)}
				class="-mr-1 rounded p-0.5 text-subtle transition-colors hover:text-fg"
				aria-label="Dismiss"
			>
				<Icon name="x" size={14} />
			</button>
		</div>
	{/each}
</div>
