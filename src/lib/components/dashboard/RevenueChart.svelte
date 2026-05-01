<script lang="ts">
	import type { MonthlyRevenue } from '$lib/types/index.js';
	import { formatCurrency } from '$lib/utils/format.js';

	interface Props {
		data: MonthlyRevenue[];
		currency?: string;
	}

	const { data, currency = 'USD' }: Props = $props();

	const chartWidth = 600;
	const chartHeight = 220;
	const paddingLeft = 60;
	const paddingRight = 16;
	const paddingTop = 16;
	const paddingBottom = 48;

	const plotWidth = chartWidth - paddingLeft - paddingRight;
	const plotHeight = chartHeight - paddingTop - paddingBottom;

	const maxRevenue = $derived(Math.max(...data.map((d) => d.revenue), 1));

	const yMax = $derived((() => {
		const mag = Math.pow(10, Math.floor(Math.log10(maxRevenue)));
		return Math.ceil(maxRevenue / mag) * mag;
	})());

	const barWidth = $derived(plotWidth / data.length);
	const barGap = $derived(Math.max(2, barWidth * 0.15));

	function barX(i: number): number {
		return paddingLeft + i * barWidth + barGap / 2;
	}

	function barH(revenue: number): number {
		return (revenue / yMax) * plotHeight;
	}

	function barY(revenue: number): number {
		return paddingTop + plotHeight - barH(revenue);
	}

	const yTicks = $derived(
		[0, 0.25, 0.5, 0.75, 1].map((f) => ({
			value: yMax * f,
			y: paddingTop + plotHeight - f * plotHeight
		}))
	);

	let hoveredIndex = $state<number | null>(null);

	function tipX(bx: number, bw: number): number {
		const tipW = 90;
		return Math.min(Math.max(bx + bw / 2 - tipW / 2, paddingLeft), chartWidth - paddingRight - tipW);
	}

	function tipY(by: number): number {
		return by - 32;
	}
</script>

<div class="relative w-full overflow-x-auto">
	<svg
		viewBox="0 0 {chartWidth} {chartHeight}"
		class="w-full"
		role="img"
		aria-label="Monthly revenue bar chart"
	>
		<!-- Y-axis gridlines and labels -->
		{#each yTicks as tick (tick.value)}
			<line
				x1={paddingLeft}
				y1={tick.y}
				x2={chartWidth - paddingRight}
				y2={tick.y}
				stroke="currentColor"
				stroke-width="0.5"
				class="text-gray-200 dark:text-gray-700"
				stroke-dasharray={tick.value === 0 ? 'none' : '4 2'}
			/>
			<text
				x={paddingLeft - 6}
				y={tick.y + 4}
				text-anchor="end"
				class="fill-gray-400 dark:fill-gray-500"
				style="font-size: 10px"
			>
				{tick.value === 0
					? '0'
					: tick.value >= 1000
						? `${(tick.value / 1000).toFixed(tick.value % 1000 === 0 ? 0 : 1)}k`
						: String(Math.round(tick.value))}
			</text>
		{/each}

		<!-- Bars -->
		{#each data as point, i (i)}
			{@const bx = barX(i)}
			{@const bw = barWidth - barGap}
			{@const bh = barH(point.revenue)}
			{@const by = barY(point.revenue)}
			{@const isHovered = hoveredIndex === i}
			{@const tx = tipX(bx, bw)}
			{@const ty = tipY(by)}

			<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
			<g
				role="graphics-symbol"
				aria-label="{point.label}: {formatCurrency(point.revenue, currency)}"
				onmouseenter={() => (hoveredIndex = i)}
				onmouseleave={() => (hoveredIndex = null)}
				onfocus={() => (hoveredIndex = i)}
				onblur={() => (hoveredIndex = null)}
				tabindex="0"
				style="outline: none; cursor: default;"
			>
				<!-- Bar -->
				{#if bh > 0}
					<rect
						x={bx}
						y={by}
						width={bw}
						height={bh}
						rx="2"
						class={isHovered
							? 'fill-primary-500 dark:fill-primary-400'
							: 'fill-primary-400 dark:fill-primary-500'}
					/>
				{:else}
					<rect
						x={bx}
						y={paddingTop + plotHeight - 1}
						width={bw}
						height={1}
						class="fill-gray-200 dark:fill-gray-700"
					/>
				{/if}

				<!-- Tooltip on hover -->
				{#if isHovered && bh > 0}
					<rect x={tx} y={ty} width={90} height={28} rx="4" class="fill-gray-900 dark:fill-gray-100" opacity="0.9" />
					<text x={tx + 45} y={ty + 12} text-anchor="middle" class="fill-white dark:fill-gray-900" style="font-size: 9px; font-weight: 500">
						{point.label}
					</text>
					<text x={tx + 45} y={ty + 23} text-anchor="middle" class="fill-white dark:fill-gray-900" style="font-size: 9px;">
						{formatCurrency(point.revenue, currency)}
					</text>
				{/if}

				<!-- X-axis label (month abbreviation) -->
				<text
					x={bx + bw / 2}
					y={paddingTop + plotHeight + 14}
					text-anchor="middle"
					class="fill-gray-500 dark:fill-gray-400"
					style="font-size: 9px"
				>
					{point.label.split(' ')[0]}
				</text>
				<text
					x={bx + bw / 2}
					y={paddingTop + plotHeight + 25}
					text-anchor="middle"
					class="fill-gray-400 dark:fill-gray-500"
					style="font-size: 8px"
				>
					{point.label.split(' ')[1]}
				</text>
			</g>
		{/each}

		<!-- Y-axis line -->
		<line
			x1={paddingLeft}
			y1={paddingTop}
			x2={paddingLeft}
			y2={paddingTop + plotHeight}
			stroke="currentColor"
			stroke-width="1"
			class="text-gray-300 dark:text-gray-600"
		/>
	</svg>
</div>
