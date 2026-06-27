<script lang="ts">
	import Button from '$lib/components/Button.svelte';
	import Badge from '$lib/components/Badge.svelte';
	import PricingCard from '$lib/components/PricingCard.svelte';
	import { pricesFor } from '$lib/pricing';
	import Receipt from '@lucide/svelte/icons/receipt';
	import FileText from '@lucide/svelte/icons/file-text';
	import Users from '@lucide/svelte/icons/users';
	import Tag from '@lucide/svelte/icons/tag';
	import Calculator from '@lucide/svelte/icons/calculator';
	import BookOpen from '@lucide/svelte/icons/book-open';

	// Display-only billing toggle — no backend effect. All CTAs go to /signup.
	let annual = $state(false);

	// Prices are display-only marketing copy; selection lives in $lib/pricing.
	let prices = $derived(pricesFor(annual));
	let period = $derived(annual ? '/mo, billed annually' : '/month');

	const starterFeatures = [
		'Up to 5 clients',
		'Unlimited invoices',
		'Estimates & quotes',
		'Product catalogue',
		'PDF export',
		'Email support'
	];
	const professionalFeatures = [
		'Unlimited clients',
		'Unlimited invoices',
		'Estimates & quotes',
		'Product catalogue',
		'Tax reports',
		'Recurring invoices',
		'Priority support'
	];
	const businessFeatures = [
		'Everything in Professional',
		'Multiple team members',
		'Advanced tax reporting',
		'Custom branding',
		'API access',
		'Dedicated support'
	];

	const features = [
		{
			icon: FileText,
			title: 'Invoices',
			description:
				'Create professional invoices in seconds. Track payment status, send reminders, and get paid faster.'
		},
		{
			icon: Calculator,
			title: 'Estimates',
			description:
				'Send polished estimates that convert. Clients approve with a click — turn quotes into invoices automatically.'
		},
		{
			icon: Users,
			title: 'Clients',
			description:
				'Keep all your client details in one place. Contact info, billing history, outstanding balances — always at hand.'
		},
		{
			icon: Tag,
			title: 'Tax',
			description:
				'Stay compliant with automated tax calculations. Run reports at year-end without the spreadsheet scramble.'
		},
		{
			icon: BookOpen,
			title: 'Catalogue',
			description:
				'Build a product and service catalogue. Add line items in one click — no retyping your most common work.'
		},
		{
			icon: Receipt,
			title: 'All in one place',
			description:
				'Everything you need to run your freelance business. No switching between apps, no double-entry, no stress.'
		}
	];

	const faqs = [
		{
			q: 'Do I need a credit card to start?',
			a: 'No. Your 90-day free trial starts the moment you sign up — no card required until you choose to subscribe.'
		},
		{
			q: 'What happens when my trial ends?',
			a: "You'll get a reminder before your trial expires. If you choose not to subscribe, your data stays safe and you can export it at any time."
		},
		{
			q: 'Can I cancel at any time?',
			a: 'Yes, absolutely. Cancel from your billing settings whenever you like. No lock-in, no cancellation fees.'
		},
		{
			q: 'Is my data secure?',
			a: 'Tallyo runs on Google Cloud with encryption at rest and in transit. Your data is yours — we never share it with third parties.'
		},
		{
			q: 'Do you support multiple currencies?',
			a: 'You can set your default currency per business. Invoice currency display is on our roadmap for later this year.'
		},
		{
			q: 'Can I import my existing clients and invoices?',
			a: 'CSV import is coming soon. In the meantime, our support team can help you migrate your data.'
		}
	];
</script>

<svelte:head>
	<title>Tallyo — Invoicing &amp; billing for freelancers</title>
	<meta
		name="description"
		content="Create invoices, send estimates, manage clients, and track tax — all in one place. Start your 90-day free trial today."
	/>
</svelte:head>

<!-- Skip to content for keyboard/screen-reader users. -->
<a
	href="#main"
	class="sr-only rounded-lg bg-brand-700 px-4 py-2 text-sm font-medium text-onbrand focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50"
>
	Skip to content
</a>

<!-- Sticky nav -->
<header
	class="sticky top-0 z-30 border-b border-gray-200 bg-white/90 backdrop-blur supports-[backdrop-filter]:bg-white/75"
>
	<div class="mx-auto flex max-w-6xl items-center justify-between px-4 py-3 sm:px-6">
		<a href="/" class="flex items-center gap-2" aria-label="Tallyo home">
			<span class="flex size-8 items-center justify-center rounded-lg bg-brand-700 text-onbrand">
				<Receipt class="size-5" aria-hidden="true" />
			</span>
			<span class="text-xl font-semibold tracking-tight text-brand-700">Tallyo</span>
		</a>

		<nav class="hidden items-center gap-6 sm:flex" aria-label="Main navigation">
			<a href="#pricing" class="text-sm font-medium text-gray-600 hover:text-gray-900">Pricing</a>
			<a href="#features" class="text-sm font-medium text-gray-600 hover:text-gray-900">Features</a>
			<a href="#faq" class="text-sm font-medium text-gray-600 hover:text-gray-900">FAQ</a>
			<a href="/login" class="text-sm font-medium text-gray-600 hover:text-gray-900">Sign in</a>
		</nav>

		<Button href="/signup" size="sm">Start free trial</Button>
	</div>

	<!-- Small screens: section anchors as a second wrapped row (no drawer). -->
	<nav
		class="flex flex-wrap items-center gap-x-5 gap-y-1 border-t border-gray-100 px-4 py-2 sm:hidden"
		aria-label="Sections"
	>
		<a href="#pricing" class="text-sm font-medium text-gray-600 hover:text-gray-900">Pricing</a>
		<a href="#features" class="text-sm font-medium text-gray-600 hover:text-gray-900">Features</a>
		<a href="#faq" class="text-sm font-medium text-gray-600 hover:text-gray-900">FAQ</a>
		<a href="/login" class="text-sm font-medium text-gray-600 hover:text-gray-900">Sign in</a>
	</nav>
</header>

<main id="main">
	<!-- Hero -->
	<section class="bg-gradient-to-b from-brand-50 to-white px-4 py-20 text-center sm:px-6 sm:py-28">
		<div class="mx-auto max-w-3xl">
			<Badge tone="brand" class="mb-4">90-day free trial · No card required</Badge>
			<h1 class="mt-4 text-4xl font-bold tracking-tight text-gray-900 sm:text-5xl lg:text-6xl">
				Invoicing &amp; billing<br />
				<span class="text-brand-700">made simple</span>
			</h1>
			<p class="mt-6 text-lg text-gray-600 sm:text-xl">
				Create invoices, send estimates, manage clients, and stay on top of tax — all in one clean
				tool built for freelancers and small businesses.
			</p>
			<div class="mt-10 flex flex-col items-center justify-center gap-4 sm:flex-row">
				<Button href="/signup" class="px-6 py-3 text-base">Start free trial</Button>
				<Button href="#pricing" variant="secondary" class="px-6 py-3 text-base">
					See pricing
				</Button>
			</div>
		</div>
	</section>

	<!-- Pricing -->
	<section id="pricing" class="bg-gray-50 px-4 py-20 sm:px-6">
		<div class="mx-auto max-w-6xl">
			<div class="mb-12 text-center">
				<h2 class="text-3xl font-bold tracking-tight text-gray-900 sm:text-4xl">
					Simple, transparent pricing
				</h2>
				<p class="mt-4 text-lg text-gray-600">Start free. Upgrade when you're ready.</p>

				<!-- Monthly / Annual toggle — display only -->
				<div class="mt-8 inline-flex items-center gap-3 rounded-lg border border-gray-200 bg-white p-1">
					<button
						type="button"
						onclick={() => (annual = false)}
						class="rounded-md px-4 py-1.5 text-sm font-medium transition-colors
							{!annual ? 'bg-brand-700 text-onbrand' : 'text-gray-600 hover:text-gray-900'}"
						aria-pressed={!annual}
					>
						Monthly
					</button>
					<button
						type="button"
						onclick={() => (annual = true)}
						class="rounded-md px-4 py-1.5 text-sm font-medium transition-colors
							{annual ? 'bg-brand-700 text-onbrand' : 'text-gray-600 hover:text-gray-900'}"
						aria-pressed={annual}
					>
						Annual
						<!-- amber bg is fixed in both themes, so pin the text dark in both. -->
						<span class="ml-1.5 rounded-full bg-accent-500 px-1.5 py-0.5 text-xs text-[#111827]">
							Save 17%
						</span>
					</button>
				</div>
				{#if annual}
					<p class="mt-2 text-sm text-gray-500">Billed annually · prices shown per month</p>
				{/if}
			</div>

			<div class="grid gap-8 sm:grid-cols-3">
				<PricingCard
					name="Starter"
					price={prices.starter}
					{period}
					description="For freelancers just getting started."
					features={starterFeatures}
				/>
				<PricingCard
					name="Professional"
					price={prices.professional}
					{period}
					description="For growing freelancers and consultants."
					features={professionalFeatures}
					popular={true}
				/>
				<PricingCard
					name="Business"
					price={prices.business}
					{period}
					description="For small agencies and teams."
					features={businessFeatures}
				/>
			</div>

			<p class="mt-8 text-center text-sm text-gray-500">
				All plans include a <strong>90-day free trial</strong> — no credit card required.
				<a href="/signup" class="font-medium text-brand-700 hover:text-brand-800"
					>Get started today →</a
				>
			</p>
		</div>
	</section>

	<!-- Features -->
	<section id="features" class="bg-white px-4 py-20 sm:px-6">
		<div class="mx-auto max-w-6xl">
			<div class="mb-12 text-center">
				<h2 class="text-3xl font-bold tracking-tight text-gray-900 sm:text-4xl">
					Everything you need to get paid
				</h2>
				<p class="mt-4 text-lg text-gray-600">
					No bloat, no complexity — just the tools that matter for your business.
				</p>
			</div>

			<ul class="grid gap-8 sm:grid-cols-2 lg:grid-cols-3" role="list">
				{#each features as feat}
					<li class="flex gap-4">
						<div
							class="flex size-10 shrink-0 items-center justify-center rounded-lg bg-brand-50 text-brand-700"
							aria-hidden="true"
						>
							<feat.icon class="size-5" />
						</div>
						<div>
							<h3 class="font-semibold text-gray-900">{feat.title}</h3>
							<p class="mt-1 text-sm text-gray-600">{feat.description}</p>
						</div>
					</li>
				{/each}
			</ul>
		</div>
	</section>

	<!-- FAQ -->
	<section id="faq" class="bg-gray-50 px-4 py-20 sm:px-6">
		<div class="mx-auto max-w-3xl">
			<div class="mb-12 text-center">
				<h2 class="text-3xl font-bold tracking-tight text-gray-900 sm:text-4xl">
					Frequently asked questions
				</h2>
			</div>

			<div class="space-y-2">
				{#each faqs as faq}
					<details class="group rounded-xl border border-gray-200 bg-white">
						<summary
							class="flex cursor-pointer list-none items-center justify-between px-5 py-4 text-sm font-medium text-gray-900 hover:text-brand-700 [&::-webkit-details-marker]:hidden"
						>
							{faq.q}
							<svg
								class="size-4 shrink-0 text-gray-400 transition-transform group-open:rotate-180 motion-reduce:transition-none"
								viewBox="0 0 20 20"
								fill="currentColor"
								aria-hidden="true"
							>
								<path
									fill-rule="evenodd"
									d="M5.22 8.22a.75.75 0 0 1 1.06 0L10 11.94l3.72-3.72a.75.75 0 1 1 1.06 1.06l-4.25 4.25a.75.75 0 0 1-1.06 0L5.22 9.28a.75.75 0 0 1 0-1.06Z"
									clip-rule="evenodd"
								/>
							</svg>
						</summary>
						<p class="border-t border-gray-100 px-5 py-4 text-sm text-gray-600">{faq.a}</p>
					</details>
				{/each}
			</div>
		</div>
	</section>

	<!-- Final CTA -->
	<section class="bg-brand-700 px-4 py-20 text-center sm:px-6">
		<div class="mx-auto max-w-2xl">
			<h2 class="text-3xl font-bold tracking-tight text-onbrand dark:text-white sm:text-4xl">
				Ready to get paid on time?
			</h2>
			<!-- In dark mode brand-700 is a lighter teal; light body copy drops below
			     4.5:1, so dark text (dark:text-white maps to near-black) is required. -->
			<p class="mt-4 text-lg text-brand-100 dark:text-white">
				Join thousands of freelancers who run their billing with Tallyo. Start your 90-day free
				trial — no credit card needed.
			</p>
			<Button href="/signup" variant="secondary" class="mt-10 px-8 py-3 text-base">
				Start free trial
			</Button>
		</div>
	</section>
</main>

<!-- Footer -->
<footer class="border-t border-gray-200 bg-white px-4 py-8 sm:px-6">
	<div class="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 sm:flex-row">
		<div class="flex items-center gap-2">
			<span class="flex size-6 items-center justify-center rounded bg-brand-700 text-onbrand">
				<Receipt class="size-3.5" aria-hidden="true" />
			</span>
			<span class="text-sm font-semibold text-brand-700">Tallyo</span>
		</div>
		<p class="text-center text-xs text-gray-500">
			© {new Date().getFullYear()} Tallyo. All rights reserved.
		</p>
		<nav class="flex gap-4" aria-label="Footer navigation">
			<a href="/login" class="text-xs text-gray-500 hover:text-gray-700">Sign in</a>
			<a href="/signup" class="text-xs text-gray-500 hover:text-gray-700">Sign up</a>
		</nav>
	</div>
</footer>
