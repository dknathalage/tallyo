import { i18n } from '$lib/stores/i18n.svelte.js';

const LOCALE_MAP: Record<string, string> = {
	en: 'en-US',
	es: 'es-ES',
	fr: 'fr-FR',
	de: 'de-DE',
	ja: 'ja-JP'
};

function getIntlLocale(locale?: string): string {
	const loc = locale ?? i18n.locale;
	return LOCALE_MAP[loc] ?? 'en-US';
}

export function formatCurrency(amount: number, currencyCode: string = 'USD', locale?: string): string {
	return new Intl.NumberFormat(getIntlLocale(locale), {
		style: 'currency',
		currency: currencyCode
	}).format(amount);
}

export function formatDate(dateStr: string, locale?: string): string {
	const date = new Date(dateStr + 'T00:00:00');
	return new Intl.DateTimeFormat(getIntlLocale(locale), {
		month: 'short',
		day: 'numeric',
		year: 'numeric'
	}).format(date);
}

export function formatDateInput(dateStr: string): string {
	const date = new Date(dateStr + 'T00:00:00');
	const year = date.getFullYear();
	const month = String(date.getMonth() + 1).padStart(2, '0');
	const day = String(date.getDate()).padStart(2, '0');
	return `${year}-${month}-${day}`;
}

export function today(): string {
	const now = new Date();
	const year = now.getFullYear();
	const month = String(now.getMonth() + 1).padStart(2, '0');
	const day = String(now.getDate()).padStart(2, '0');
	return `${year}-${month}-${day}`;
}
