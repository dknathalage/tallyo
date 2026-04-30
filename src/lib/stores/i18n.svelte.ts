import en from '$lib/i18n/en.js';
import type { Messages } from '$lib/i18n/types.js';

const LOCALE_KEY = 'locale';

const LOCALE_MAP: Record<string, string> = {
	en: 'en-US',
	es: 'es-ES',
	fr: 'fr-FR',
	de: 'de-DE',
	ja: 'ja-JP'
};

class I18nStore {
	locale = $state('en');
	private messages: Messages = $state(en);

	init() {
		const stored = localStorage.getItem(LOCALE_KEY);
		if (stored) {
			this.locale = stored;
		}
	}

	async setLocale(locale: string) {
		this.locale = locale;
		localStorage.setItem(LOCALE_KEY, locale);
		// Future: dynamically import locale files
		// const mod = await import(`$lib/i18n/${locale}.js`);
		// this.messages = mod.default;
	}

	get intlLocale(): string {
		return LOCALE_MAP[this.locale] ?? 'en-US';
	}

	t(key: string, values?: Record<string, string | number>): string {
		const parts = key.split('.');
		let result: unknown = this.messages;
		for (const part of parts) {
			if (result && typeof result === 'object') {
				result = (result as Record<string, unknown>)[part];
			} else {
				return key;
			}
		}
		if (typeof result !== 'string') return key;
		if (values) {
			return result.replace(/\{(\w+)\}/g, (_, k) => String(values[k] ?? `{${k}}`));
		}
		return result;
	}
}

export const i18n = new I18nStore();
