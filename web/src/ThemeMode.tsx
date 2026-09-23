import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { getPalette, type Palette, type ThemeMode } from './theme'
import { loadThemePreference, saveThemePreference, resolveTheme, watchSystemTheme, SYSTEM_THEME_QUERY, type ThemePreference } from './themePreference'

interface DesktopRuntime {
  EventsEmit: (name: string, ...data: unknown[]) => void
  EventsOn: (name: string, callback: () => void) => () => void
}

interface ThemeModeState {
  mode: ThemeMode
  preference: ThemePreference
  setPreference: (preference: ThemePreference) => void
}

const ThemeModeContext = createContext<ThemeModeState | null>(null)

export function ThemeModeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreference] = useState<ThemePreference>(loadThemePreference)
  const [systemDark, setSystemDark] = useState(() =>
    typeof window !== 'undefined' && !!window.matchMedia?.(SYSTEM_THEME_QUERY).matches,
  )
  const mode = resolveTheme(preference, systemDark)

  useEffect(() => watchSystemTheme(setSystemDark), [])
  useEffect(() => saveThemePreference(preference), [preference])

  useEffect(() => {
    // Lets native form controls / scrollbars follow the chosen mode too,
    // not just the antd-rendered parts of the page.
    document.documentElement.style.colorScheme = mode
    document.documentElement.dataset.theme = mode

    const runtime = (window as Window & { runtime?: DesktopRuntime }).runtime
    if (!runtime?.EventsEmit || !runtime?.EventsOn) return
    const syncTheme = () => runtime.EventsEmit('desktop:theme', mode, preference)
    const unsubscribe = runtime.EventsOn('desktop:theme-ready', syncTheme)
    // The ready event covers startup ordering; immediate sync also covers reloads and toggles.
    syncTheme()
    return unsubscribe
  }, [mode, preference])

  const value = useMemo<ThemeModeState>(
    () => ({ mode, preference, setPreference }),
    [mode, preference],
  )

  return <ThemeModeContext.Provider value={value}>{children}</ThemeModeContext.Provider>
}

export function useThemeMode(): ThemeModeState {
  const ctx = useContext(ThemeModeContext)
  if (!ctx) throw new Error('useThemeMode must be used within a ThemeModeProvider')
  return ctx
}

export function usePalette(): Palette {
  const { mode } = useThemeMode()
  return getPalette(mode)
}
