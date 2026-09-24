import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { LANGUAGE_STORAGE_KEY, loadLanguagePreference, resolveLanguage, translator, type Language, type LanguagePreference, type Translate } from './locale'

interface I18nState {
  language: Language
  locale: string
  preference: LanguagePreference
  setPreference: (preference: LanguagePreference) => void
  tr: Translate
}

const I18nContext = createContext<I18nState | null>(null)

export function I18nProvider({ children }: { children: ReactNode }) {
  const [preference, setPreference] = useState(loadLanguagePreference)
  const [languages, setLanguages] = useState(() => navigator.languages?.length ? navigator.languages : [navigator.language])
  const language = resolveLanguage(preference, languages)
  useEffect(() => {
    const update = () => setLanguages(navigator.languages?.length ? navigator.languages : [navigator.language])
    window.addEventListener('languagechange', update)
    const storage = (event: StorageEvent) => {
      if (event.key === LANGUAGE_STORAGE_KEY || event.key === null) setPreference(loadLanguagePreference())
    }
    window.addEventListener('storage', storage)
    return () => {
      window.removeEventListener('languagechange', update)
      window.removeEventListener('storage', storage)
    }
  }, [])
  useEffect(() => {
    try { window.localStorage.setItem(LANGUAGE_STORAGE_KEY, preference) } catch { /* Session-only preference. */ }
  }, [preference])
  useEffect(() => {
    document.documentElement.lang = language === 'zh' ? 'zh-CN' : 'en'
    const runtime = (window as Window & { runtime?: {
      EventsEmit: (event: string, ...data: unknown[]) => void
      EventsOn: (event: string, callback: () => void) => () => void
    } }).runtime
    if (!runtime?.EventsEmit || !runtime?.EventsOn) return
    const sync = () => runtime.EventsEmit('desktop:language', language)
    const unsubscribe = runtime.EventsOn('desktop:language-ready', sync)
    sync()
    return unsubscribe
  }, [language])
  const value = useMemo(() => ({ language, locale: language === 'zh' ? 'zh-CN' : 'en-US', preference, setPreference, tr: translator(language) }), [language, preference])
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

export function useI18n(): I18nState {
  const value = useContext(I18nContext)
  if (!value) throw new Error('useI18n requires I18nProvider')
  return value
}
