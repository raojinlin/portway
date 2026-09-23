import type { ThemeMode } from './theme'

export type ThemePreference = 'system' | ThemeMode
export const THEME_STORAGE_KEY = 'stm-theme-mode'
export const SYSTEM_THEME_QUERY = '(prefers-color-scheme: dark)'

export function loadThemePreference(): ThemePreference {
  try {
    const saved = window.localStorage.getItem(THEME_STORAGE_KEY)
    if (saved === 'light' || saved === 'dark') return saved
  } catch {
    // Private browsing or storage policies must not prevent the app from loading.
  }
  return 'system'
}

export function saveThemePreference(preference: ThemePreference): void {
  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, preference)
  } catch {
    // The current session can still use the selected theme without persistence.
  }
}

export function resolveTheme(preference: ThemePreference, systemDark: boolean): ThemeMode {
  return preference === 'system' ? (systemDark ? 'dark' : 'light') : preference
}

export function watchSystemTheme(update: (dark: boolean) => void): () => void {
  const query = window.matchMedia?.(SYSTEM_THEME_QUERY)
  if (!query) return () => {}
  const onChange = () => update(query.matches)
  query.addEventListener('change', onChange)
  onChange()
  return () => query.removeEventListener('change', onChange)
}
