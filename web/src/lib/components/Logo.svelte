<script lang="ts">
	import { APP_NAME } from '$lib/brand';

	let { size = 28 }: { size?: number } = $props();
</script>

<span class="inline-flex items-center gap-2.5">
	<span
		class="grid place-items-center rounded-lg bg-accent text-white shadow-[0_0_0_1px_rgba(255,255,255,0.12)_inset]"
		style="width:{size}px;height:{size}px"
	>
		<svg
			viewBox="0 0 24 24"
			width={size * 0.58}
			height={size * 0.58}
			fill="none"
			stroke="currentColor"
			stroke-width="2.4"
			stroke-linecap="round"
			stroke-linejoin="round"
			aria-hidden="true"
		>
			<path d="M4 12h10" />
			<path d="m10 6 6 6-6 6" />
			<path d="M20 5v14" />
		</svg>
	</span>
	<span class="text-[15px] font-semibold tracking-tight">{APP_NAME}</span>
</span>
