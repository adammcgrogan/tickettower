<script lang="ts">
	import { APP_NAME } from '$lib/brand';

	let { size = 28 }: { size?: number } = $props();
</script>

<span class="inline-flex items-center gap-2.5">
	<span class="grid place-items-center rounded-lg bg-accent text-on-accent" style="width:{size}px;height:{size}px">
		<svg viewBox="0 0 24 24" width={size * 0.8} height={size * 0.8} fill="currentColor" aria-hidden="true">
			<path
				fill-rule="evenodd"
				d="M4.5 2.5h3.4v1.7h2.4V2.5h3.4v1.7h2.4V2.5h3.4v5.1a1.5 1.5 0 0 1-1.5 1.5h-.6v12.4H6.6V9.1H6a1.5 1.5 0 0 1-1.5-1.5zm5.4 14.3h4.2v-3a2.1 2.1 0 0 0-4.2 0z"
			/>
		</svg>
	</span>
	<span class="font-display text-[22px] leading-none font-extrabold tracking-[0.01em]">{APP_NAME}</span>
</span>
