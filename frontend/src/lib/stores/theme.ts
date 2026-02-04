import { writable } from 'svelte/store';
import { ThemePreference } from '$api/hookly/v1/common_pb';

export type Theme = 'system' | 'light' | 'dark' | 'placid-blue-light' | 'placid-blue-dark';

// Convert proto enum to theme string
export function themePreferenceToString(pref: ThemePreference): Theme {
	switch (pref) {
		case ThemePreference.SYSTEM:
			return 'system';
		case ThemePreference.LIGHT:
			return 'light';
		case ThemePreference.DARK:
			return 'dark';
		case ThemePreference.PLACID_BLUE_LIGHT:
			return 'placid-blue-light';
		case ThemePreference.PLACID_BLUE_DARK:
			return 'placid-blue-dark';
		default:
			return 'system';
	}
}

// Convert theme string to proto enum
export function stringToThemePreference(theme: Theme): ThemePreference {
	switch (theme) {
		case 'system':
			return ThemePreference.SYSTEM;
		case 'light':
			return ThemePreference.LIGHT;
		case 'dark':
			return ThemePreference.DARK;
		case 'placid-blue-light':
			return ThemePreference.PLACID_BLUE_LIGHT;
		case 'placid-blue-dark':
			return ThemePreference.PLACID_BLUE_DARK;
		default:
			return ThemePreference.SYSTEM;
	}
}

function createThemeStore() {
	const { subscribe, set, update } = writable<Theme>('system');

	return {
		subscribe,
		set: (theme: Theme) => {
			// Apply theme to document
			if (typeof document !== 'undefined') {
				document.documentElement.setAttribute('data-theme', theme);
			}
			set(theme);
		},
		initialize: (serverTheme: ThemePreference) => {
			const theme = themePreferenceToString(serverTheme);
			// Apply theme to document
			if (typeof document !== 'undefined') {
				document.documentElement.setAttribute('data-theme', theme);
			}
			set(theme);
		}
	};
}

export const theme = createThemeStore();
