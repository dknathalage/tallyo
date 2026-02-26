type ThemePreference = 'light' | 'dark' | 'system';

const STORAGE_KEY = 'theme';

function createThemeStore() {
	let preference = $state<ThemePreference>('system');
	let isDark = $state(false);

	function applyTheme(dark: boolean) {
		isDark = dark;
		if (dark) {
			document.documentElement.classList.add('dark');
		} else {
			document.documentElement.classList.remove('dark');
		}
	}

	function resolveAndApply() {
		if (preference === 'system') {
			applyTheme(window.matchMedia('(prefers-color-scheme: dark)').matches);
		} else {
			applyTheme(preference === 'dark');
		}
	}

	function init() {
		const stored = localStorage.getItem(STORAGE_KEY) as ThemePreference | null;
		if (stored === 'light' || stored === 'dark' || stored === 'system') {
			preference = stored;
		}
		resolveAndApply();

		window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
			if (preference === 'system') {
				resolveAndApply();
			}
		});
	}

	function set(pref: ThemePreference) {
		preference = pref;
		localStorage.setItem(STORAGE_KEY, pref);
		resolveAndApply();
	}

	function toggle() {
		set(isDark ? 'light' : 'dark');
	}

	return {
		get preference() {
			return preference;
		},
		get isDark() {
			return isDark;
		},
		init,
		set,
		toggle
	};
}

export const theme = createThemeStore();
