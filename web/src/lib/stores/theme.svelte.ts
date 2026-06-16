/**
 * Theme store: toggles a `dark` class on <html>. The actual colour swap happens
 * in app.css, which redefines Tailwind's gray/white CSS variables under `.dark`,
 * so every existing utility (bg-white, text-gray-900, …) flips automatically.
 *
 * An inline script in app.html applies the stored/system theme before hydration
 * to avoid a flash; init() just syncs this store's reactive state to that result.
 */
const STORAGE_KEY = 'tallyo-theme';
type Theme = 'light' | 'dark';

function isTheme(v: string | null): v is Theme {
	return v === 'light' || v === 'dark';
}

function createThemeStore() {
	let theme = $state<Theme>('light');

	function apply(t: Theme): void {
		if (typeof document === 'undefined') return;
		document.documentElement.classList.toggle('dark', t === 'dark');
	}

	function init(): void {
		if (typeof window === 'undefined') return;
		const stored = localStorage.getItem(STORAGE_KEY);
		if (isTheme(stored)) {
			theme = stored;
		} else {
			theme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
		}
		apply(theme);
	}

	function toggle(): void {
		theme = theme === 'dark' ? 'light' : 'dark';
		localStorage.setItem(STORAGE_KEY, theme);
		apply(theme);
	}

	return {
		get current(): Theme {
			return theme;
		},
		get isDark(): boolean {
			return theme === 'dark';
		},
		init,
		toggle
	};
}

export const theme = createThemeStore();
