<script lang="ts">
	/**
	 * A ticket number shaped like an admission stub. Amber means the ticket is
	 * waiting on the team; it's the one loud element in the interface.
	 */
	let {
		number,
		tone = 'default',
		size = 'md'
	}: { number: number; tone?: 'waiting' | 'default' | 'closed'; size?: 'sm' | 'md' | 'lg' } = $props();

	const sizes = {
		sm: 'h-6 min-w-14 px-2 text-[15px]',
		md: 'h-8 min-w-[4.5rem] px-2.5 text-xl',
		lg: 'h-14 min-w-32 px-4 text-4xl [--notch:7px]'
	};
	const tones = {
		waiting: 'bg-accent text-on-accent',
		default: 'bg-elevated text-fg',
		closed: 'bg-elevated/60 text-subtle'
	};
</script>

<span class="stub font-display leading-none font-bold tabular-nums {sizes[size]} {tones[tone]}">
	<span class="sr-only">Ticket {number}</span>
	<span aria-hidden="true">#{String(number).padStart(4, '0')}</span>
</span>
