// Three-way theme preference (system/light/dark), remembered in localStorage
// and applied as data-theme on <html>. app.html runs the same logic inline,
// before first paint, so there's no flash of the wrong theme; this module
// keeps the sidebar toggle in sync and reacts to a live OS preference change.

export type ThemePreference = 'system' | 'light' | 'dark';

const STORAGE_KEY = 'theme';
const LIGHT_THEME_COLOR = '#f5f7fa';
const DARK_THEME_COLOR = '#0e1522';

function readStored(): ThemePreference {
	try {
		const v = localStorage.getItem(STORAGE_KEY);
		if (v === 'light' || v === 'dark') return v;
	} catch {
		// Private browsing, or storage blocked: fall back to system.
	}
	return 'system';
}

export const themeState = $state<{ preference: ThemePreference }>({ preference: 'system' });

function apply(preference: ThemePreference) {
	const root = document.documentElement;
	if (preference === 'system') root.removeAttribute('data-theme');
	else root.setAttribute('data-theme', preference);

	const light =
		preference === 'light' ||
		(preference === 'system' && window.matchMedia('(prefers-color-scheme: light)').matches);
	document.querySelector('meta[name="theme-color"]')?.setAttribute('content', light ? LIGHT_THEME_COLOR : DARK_THEME_COLOR);
}

export function setTheme(preference: ThemePreference) {
	themeState.preference = preference;
	try {
		if (preference === 'system') localStorage.removeItem(STORAGE_KEY);
		else localStorage.setItem(STORAGE_KEY, preference);
	} catch {
		// Ignore: the toggle still works for this session.
	}
	apply(preference);
}

// Called once, from the root layout, to sync the toggle's state with what
// app.html's inline script already applied, and keep "System" live.
export function initTheme() {
	themeState.preference = readStored();
	const system = window.matchMedia('(prefers-color-scheme: light)');
	system.addEventListener('change', () => {
		if (themeState.preference === 'system') apply('system');
	});
}
