<script lang="ts">
	// Gold sparkles icon (Lucide "sparkles" path). On hover a bright band travels
	// across it like a blade of light. The gradient is clipped to the icon via a
	// mask; the band is a moving background-position.
	let { class: cls = '' }: { class?: string } = $props();
</script>

<span class="blade {cls}" aria-hidden="true"></span>

<style>
	.blade {
		display: inline-block;
		width: 1em;
		height: 1em;
		vertical-align: -0.125em;
		/* Gold base with a bright band; band sits off to the right at rest. */
		background-image: linear-gradient(
			115deg,
			#c8961a 0%,
			#c8961a 42%,
			#fff3c4 50%,
			#c8961a 58%,
			#c8961a 100%
		);
		background-size: 300% 100%;
		background-position: 130% 0;
		transition: background-position 0.65s ease;
		-webkit-mask: var(--blade-mask) center / contain no-repeat;
		mask: var(--blade-mask) center / contain no-repeat;
		--blade-mask: url("data:image/svg+xml,%3Csvg%20xmlns='http://www.w3.org/2000/svg'%20viewBox='0%200%2024%2024'%20fill='none'%20stroke='black'%20stroke-width='2'%20stroke-linecap='round'%20stroke-linejoin='round'%3E%3Cpath%20d='M9.937%2015.5A2%202%200%200%200%208.5%2014.063l-6.135-1.582a.5.5%200%200%201%200-.962L8.5%209.936A2%202%200%200%200%209.937%208.5l1.582-6.135a.5.5%200%200%201%20.962%200L14.063%208.5A2%202%200%200%200%2015.5%209.937l6.135%201.581a.5.5%200%200%201%200%20.964L15.5%2014.063a2%202%200%200%200-1.437%201.437l-1.582%206.135a.5.5%200%200%201-.962%200z'/%3E%3Cpath%20d='M20%203v4'/%3E%3Cpath%20d='M22%205h-4'/%3E%3Cpath%20d='M4%2017v2'/%3E%3Cpath%20d='M5%2018H3'/%3E%3C/svg%3E");
	}
	.blade:hover,
	:global(button:hover) .blade,
	:global(label:hover) .blade {
		background-position: -30% 0;
	}
	@media (prefers-reduced-motion: reduce) {
		.blade {
			transition: none;
		}
	}
</style>
